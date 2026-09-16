package mjpeg

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/ascii"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/mpjpeg"
	"github.com/AlexxIT/go2rtc/pkg/y4m"
	"github.com/rs/zerolog"
)

func Init() {
	api.HandleFunc("api/frame.jpeg", handlerKeyframe)
	api.HandleFunc("api/stream.mjpeg", handlerStream)
	api.HandleFunc("api/stream.ascii", handlerStream)
	api.HandleFunc("api/stream.y4m", apiStreamY4M)

	ws.HandleFunc("mjpeg", handlerWS)

	log = app.GetLogger("mjpeg")
}

var log zerolog.Logger

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stream, _ := streams.GetOrPatch(query)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	var b []byte

	if s := query.Get("cache"); s != "" {
		if timeout, err := time.ParseDuration(s); err == nil {
			src := query.Get("src")

			cacheMu.Lock()
			entry, found := cache[src]
			cacheMu.Unlock()

			if found && time.Since(entry.timestamp) < timeout {
				writeJPEGResponse(w, entry.payload)
				return
			}

			defer func() {
				if b == nil {
					return
				}
				entry = cacheEntry{payload: b, timestamp: time.Now()}
				cacheMu.Lock()
				if cache == nil {
					cache = map[string]cacheEntry{src: entry}
				} else {
					cache[src] = entry
				}
				cacheMu.Unlock()
			}()
		}
	}

	cons := magic.NewKeyframe()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// handle client disconnect and cameras that never send a keyframe,
	// otherwise WriteTo below waits forever and the producer can't be stopped
	ctx, cancel := context.WithTimeout(r.Context(), keyframeTimeout(query))
	defer cancel()

	var removeOnce sync.Once
	remove := func() { removeOnce.Do(func() { stream.RemoveConsumer(cons) }) }
	defer remove()

	go func() {
		<-ctx.Done()
		remove()
	}()

	once := &core.OnceBuffer{} // init and first frame
	_, _ = cons.WriteTo(once)
	b = once.Buffer()

	remove()

	if len(b) == 0 {
		log.Warn().Str("src", query.Get("src")).Msgf("[mjpeg] no keyframe: %s", ctx.Err())
		http.Error(w, "timeout waiting for keyframe", http.StatusGatewayTimeout)
		return
	}

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		ts := time.Now()
		var err error
		if b, err = ffmpeg.JPEGWithQuery(b, query); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Debug().Msgf("[mjpeg] transcoding time=%s", time.Since(ts))
	case core.CodecJPEG:
		b = mjpeg.FixJPEG(b)
	}

	writeJPEGResponse(w, b)
}

// defaultKeyframeTimeout caps how long a snapshot request waits for a keyframe.
// Should be above the camera GOP interval, but low enough to not pile up requests.
const defaultKeyframeTimeout = 10 * time.Second

// keyframeTimeout reads the optional `timeout` query param (seconds).
func keyframeTimeout(query url.Values) time.Duration {
	if i := core.Atoi(query.Get("timeout")); i > 0 {
		return time.Duration(i) * time.Second
	}
	return defaultKeyframeTimeout
}

var cache map[string]cacheEntry
var cacheMu sync.Mutex

// cacheEntry represents a cached keyframe with its timestamp
type cacheEntry struct {
	payload   []byte
	timestamp time.Time
}

func writeJPEGResponse(w http.ResponseWriter, b []byte) {
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Content-Length", strconv.Itoa(len(b)))
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		outputMjpeg(w, r)
	} else {
		inputMjpeg(w, r)
	}
}

func outputMjpeg(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := mjpeg.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.mjpeg] add consumer")
		return
	}

	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	if strings.HasSuffix(r.URL.Path, "mjpeg") {
		wr := mjpeg.NewWriter(w)
		_, _ = cons.WriteTo(wr)
	} else {
		cons.FormatName = "ascii"

		query := r.URL.Query()
		wr := ascii.NewWriter(w, query.Get("color"), query.Get("back"), query.Get("text"))
		_, _ = cons.WriteTo(wr)
	}

	stream.RemoveConsumer(cons)
}

func inputMjpeg(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	prod, _ := mpjpeg.Open(r.Body)
	prod.WithRequest(r)

	stream.AddProducer(prod)

	if err := prod.Start(); err != nil && err != io.EOF {
		log.Warn().Err(err).Caller().Send()
	}

	stream.RemoveProducer(prod)
}

func handlerWS(tr *ws.Transport, _ *ws.Message) error {
	stream, _ := streams.GetOrPatch(tr.Request.URL.Query())
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := mjpeg.NewConsumer()
	cons.WithRequest(tr.Request)

	if err := stream.AddConsumer(cons); err != nil {
		log.Debug().Err(err).Msg("[mjpeg] add consumer")
		return err
	}

	tr.Write(&ws.Message{Type: "mjpeg"})

	go cons.WriteTo(tr.Writer())

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}

func apiStreamY4M(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := y4m.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	_, _ = cons.WriteTo(w)

	stream.RemoveConsumer(cons)
}
