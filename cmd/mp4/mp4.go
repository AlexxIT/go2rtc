package mp4

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/rs/zerolog"
	"net/http"
	"strconv"
	"strings"
)

func Init() {
	log = app.GetLogger("mp4")

	api.HandleWS(MsgTypeMSE, handlerWS)

	api.HandleFunc("/api/frame.mp4", handlerKeyframe)
	api.HandleFunc("/api/stream.mp4", handlerMP4)
}

var log zerolog.Logger

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	if isChromeFirst(w, r) {
		return
	}

	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return
	}

	exit := make(chan []byte)

	cons := &mp4.Consumer{}
	cons.Listen(func(msg interface{}) {
		switch msg := msg.(type) {
		case []byte:
			exit <- msg
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.keyframe] add consumer")
		return
	}

	defer stream.RemoveConsumer(cons)

	w.Header().Set("Content-Type", cons.MimeType())

	data, err := cons.Init()
	if err != nil {
		log.Error().Err(err).Msg("[api.keyframe] init")
		return
	}
	data = append(data, <-exit...)

	// Apple Safari won't show frame without length
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Msg("[api.keyframe] add consumer")
	}
}

func handlerMP4(w http.ResponseWriter, r *http.Request) {
	if isChromeFirst(w, r) || isSafari(w, r) {
		return
	}

	log.Trace().Msgf("[api.mp4] %+v", r)

	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return
	}

	exit := make(chan struct{})

	cons := &mp4.Consumer{}
	cons.Listen(func(msg interface{}) {
		switch msg := msg.(type) {
		case []byte:
			if _, err := w.Write(msg); err != nil {
				exit <- struct{}{}
			}
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Msg("[api.mp4] add consumer")
		return
	}

	defer stream.RemoveConsumer(cons)

	w.Header().Set("Content-Type", cons.MimeType())

	data, err := cons.Init()
	if err != nil {
		log.Error().Err(err).Msg("[api.mp4] init")
		return
	}

	if _, err = w.Write(data); err != nil {
		log.Error().Err(err).Msg("[api.mp4] write")
		return
	}

	<-exit

	log.Trace().Msg("[api.mp4] close")
}

func isChromeFirst(w http.ResponseWriter, r *http.Request) bool {
	// Chrome 105 does two requests: without Range and with `Range: bytes=0-`
	if strings.Contains(r.UserAgent(), " Chrome/") {
		if r.Header.Values("Range") == nil {
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
			return true
		}
	}
	return false
}

func isSafari(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Range") == "bytes=0-1" {
		handlerKeyframe(w, r)
		return true
	}
	return false
}
