package webp

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/webp"
	"github.com/rs/zerolog"
)

func Init() {
	api.HandleFunc("api/frame.webp", handlerKeyframe)
	api.HandleFunc("api/stream.webp", handlerStream)

	log = app.GetLogger("webp")
}

var log zerolog.Logger

var cache map[string]cacheEntry
var cacheMu sync.Mutex

type cacheEntry struct {
	payload   []byte
	timestamp time.Time
}

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	stream, _ := streams.GetOrPatch(query)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	quality := 75
	if s := query.Get("quality"); s != "" {
		if q, err := strconv.Atoi(s); err == nil && q > 0 && q <= 100 {
			quality = q
		}
	}

	var b []byte

	if s := query.Get("cache"); s != "" {
		if timeout, err := time.ParseDuration(s); err == nil {
			src := query.Get("src")

			cacheMu.Lock()
			entry, found := cache[src]
			cacheMu.Unlock()

			if found && time.Since(entry.timestamp) < timeout {
				writeWebPResponse(w, entry.payload)
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
		return
	}

	once := &core.OnceBuffer{}
	_, _ = cons.WriteTo(once)
	b = once.Buffer()

	stream.RemoveConsumer(cons)

	var err error
	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		ts := time.Now()
		var jpegBytes []byte
		if jpegBytes, err = ffmpeg.JPEGWithQuery(b, query); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Debug().Msgf("[webp] transcoding time=%s", time.Since(ts))
		if b, err = webp.EncodeJPEG(jpegBytes, quality); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case core.CodecJPEG:
		fixed := mjpeg.FixJPEG(b)
		if b, err = webp.EncodeJPEG(fixed, quality); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	writeWebPResponse(w, b)
}

func writeWebPResponse(w http.ResponseWriter, b []byte) {
	h := w.Header()
	h.Set("Content-Type", "image/webp")
	h.Set("Content-Length", strconv.Itoa(len(b)))
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	if _, err := w.Write(b); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerStream(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := webp.NewConsumer()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.webp] add consumer")
		return
	}

	h := w.Header()
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	wr := webp.NewWriter(w)
	_, _ = cons.WriteTo(wr)

	stream.RemoveConsumer(cons)
}
