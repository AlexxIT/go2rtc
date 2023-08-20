package mjpeg

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog/log"
)

func Init() {
	api.HandleFunc("api/frame.jpeg", handlerKeyframe)
	api.HandleFunc("api/stream.mjpeg", handlerStream)

	ws.HandleFunc("mjpeg", handlerWS)
}

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	exit := make(chan []byte)

	cons := &magic.Keyframe{
		RemoteAddr: tcp.RemoteAddr(r),
		UserAgent:  r.UserAgent(),
	}
	cons.Listen(func(msg any) {
		if b, ok := msg.([]byte); ok {
			select {
			case exit <- b:
			default:
			}
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	data := <-exit

	stream.RemoveConsumer(cons)

	switch cons.CodecName() {
	case core.CodecH264, core.CodecH265:
		ts := time.Now()
		var err error
		if data, err = ffmpeg.TranscodeToJPEG(data, r.URL.Query()); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Debug().Msgf("[mjpeg] transcoding time=%s", time.Since(ts))
	}

	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	if _, err := w.Write(data); err != nil {
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

	cons := &mjpeg.Consumer{
		RemoteAddr: tcp.RemoteAddr(r),
		UserAgent:  r.UserAgent(),
	}

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.mjpeg] add consumer")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	wr := &writer{wr: w, buf: []byte(header)}
	_, _ = cons.WriteTo(wr)

	stream.RemoveConsumer(cons)
}

const header = "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: "

type writer struct {
	wr  io.Writer
	buf []byte
}

func (w *writer) Write(p []byte) (n int, err error) {
	w.buf = w.buf[:len(header)]
	w.buf = append(w.buf, strconv.Itoa(len(p))...)
	w.buf = append(w.buf, "\r\n\r\n"...)
	w.buf = append(w.buf, p...)
	w.buf = append(w.buf, "\r\n"...)

	// Chrome bug: mjpeg image always shows the second to last image
	// https://bugs.chromium.org/p/chromium/issues/detail?id=527446
	if n, err = w.wr.Write(w.buf); err == nil {
		w.wr.(http.Flusher).Flush()
	}

	return
}

func inputMjpeg(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	res := &http.Response{Body: r.Body, Header: r.Header, Request: r}
	res.Header.Set("Content-Type", "multipart/mixed;boundary=")

	client := mjpeg.NewClient(res)
	stream.AddProducer(client)

	if err := client.Start(); err != nil && err != io.EOF {
		log.Warn().Err(err).Caller().Send()
	}

	stream.RemoveProducer(client)
}

func handlerWS(tr *ws.Transport, _ *ws.Message) error {
	stream := streams.GetOrPatch(tr.Request.URL.Query())
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := &mjpeg.Consumer{
		RemoteAddr: tcp.RemoteAddr(tr.Request),
		UserAgent:  tr.Request.UserAgent(),
	}

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	tr.Write(&ws.Message{Type: "mjpeg"})

	go cons.WriteTo(tr.Writer())

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}
