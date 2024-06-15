package mjpeg

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
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
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := magic.NewKeyframe()
	cons.WithRequest(r)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	once := &core.OnceBuffer{} // init and first frame
	_, _ = cons.WriteTo(once)
	b := once.Buffer()

	stream.RemoveConsumer(cons)

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		ts := time.Now()
		var err error
		if b, err = ffmpeg.JPEGWithQuery(b, r.URL.Query()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Debug().Msgf("[mjpeg] transcoding time=%s", time.Since(ts))
	case core.CodecJPEG:
		b = mjpeg.FixJPEG(b)
	}

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
	stream := streams.GetOrPatch(tr.Request.URL.Query())
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
