package mjpeg

import (
	"errors"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"strconv"
)

func Init() {
	api.HandleFunc("api/frame.jpeg", handlerKeyframe)
	api.HandleFunc("api/stream.mjpeg", handlerStream)

	api.HandleWS("mjpeg", handlerWS)
}

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	exit := make(chan []byte)

	cons := &mjpeg.Consumer{
		RemoteAddr: tcp.RemoteAddr(r),
		UserAgent:  r.UserAgent(),
	}
	cons.Listen(func(msg any) {
		switch msg := msg.(type) {
		case []byte:
			exit <- msg
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	data := <-exit

	stream.RemoveConsumer(cons)

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

const header = "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: "

func handlerStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		outputMjpeg(w, r)
	} else {
		inputMjpeg(w, r)
	}
}

func outputMjpeg(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	flusher := w.(http.Flusher)

	cons := &mjpeg.Consumer{
		RemoteAddr: tcp.RemoteAddr(r),
		UserAgent:  r.UserAgent(),
	}
	cons.Listen(func(msg any) {
		switch msg := msg.(type) {
		case []byte:
			data := []byte(header + strconv.Itoa(len(msg)))
			data = append(data, '\r', '\n', '\r', '\n')
			data = append(data, msg...)
			data = append(data, '\r', '\n')

			// Chrome bug: mjpeg image always shows the second to last image
			// https://bugs.chromium.org/p/chromium/issues/detail?id=527446
			_, _ = w.Write(data)
			flusher.Flush()
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.mjpeg] add consumer")
		return
	}

	h := w.Header()
	h.Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "close")
	h.Set("Pragma", "no-cache")

	<-r.Context().Done()

	stream.RemoveConsumer(cons)

	//log.Trace().Msg("[api.mjpeg] close")
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

func handlerWS(tr *api.Transport, _ *api.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := &mjpeg.Consumer{
		RemoteAddr: tcp.RemoteAddr(tr.Request),
		UserAgent:  tr.Request.UserAgent(),
	}
	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			tr.Write(data)
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	tr.Write(&api.Message{Type: "mjpeg"})

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}
