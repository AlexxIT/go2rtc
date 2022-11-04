package mjpeg

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"
)

func Init() {
	api.HandleFunc("api/frame.jpeg", handlerKeyframe)
	api.HandleFunc("api/stream.mjpeg", handlerStream)
}

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		return
	}

	exit := make(chan []byte)

	cons := &mjpeg.Consumer{}
	cons.Listen(func(msg interface{}) {
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

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

const header = "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: "

func handlerStream(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		return
	}

	exit := make(chan struct{})

	cons := &mjpeg.Consumer{}
	cons.Listen(func(msg interface{}) {
		switch msg := msg.(type) {
		case []byte:
			data := []byte(header + strconv.Itoa(len(msg)))
			data = append(data, 0x0D, 0x0A, 0x0D, 0x0A)
			data = append(data, msg...)
			data = append(data, 0x0D, 0x0A)

			if _, err := w.Write(data); err != nil {
				exit <- struct{}{}
			}
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.mjpeg] add consumer")
		return
	}

	w.Header().Set("Content-Type", `multipart/x-mixed-replace; boundary=frame`)

	<-exit

	stream.RemoveConsumer(cons)

	//log.Trace().Msg("[api.mjpeg] close")
}
