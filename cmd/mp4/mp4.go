package mp4

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/rs/zerolog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func Init() {
	log = app.GetLogger("mp4")

	api.HandleWS("mse", handlerWSMSE)
	api.HandleWS("mp4", handlerWSMP4)

	api.HandleFunc("api/frame.mp4", handlerKeyframe)
	api.HandleFunc("api/stream.mp4", handlerMP4)
}

var log zerolog.Logger

func handlerKeyframe(w http.ResponseWriter, r *http.Request) {
	// Chrome 105 does two requests: without Range and with `Range: bytes=0-`
	ua := r.UserAgent()
	if strings.Contains(ua, " Chrome/") {
		if r.Header.Values("Range") == nil {
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	src := r.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	exit := make(chan []byte)

	cons := &mp4.Segment{OnlyKeyframe: true}
	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok && exit != nil {
			exit <- data
			exit = nil
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	data := <-exit

	stream.RemoveConsumer(cons)

	// Apple Safari won't show frame without length
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Content-Type", cons.MimeType)

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerMP4(w http.ResponseWriter, r *http.Request) {
	log.Trace().Msgf("[mp4] %s %+v", r.Method, r.Header)

	// Chrome has Safari in UA, so check first Chrome and later Safari
	ua := r.UserAgent()
	if strings.Contains(ua, " Chrome/") {
		if r.Header.Values("Range") == nil {
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
			return
		}
	} else if strings.Contains(ua, " Safari/") {
		// auto redirect to HLS/fMP4 format, because Safari not support MP4 stream
		url := "stream.m3u8?" + r.URL.RawQuery
		if !r.URL.Query().Has("mp4") {
			url += "&mp4"
		}

		http.Redirect(w, r, url, http.StatusMovedPermanently)
		return
	}

	src := r.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	exit := make(chan error)

	cons := &mp4.Consumer{
		RemoteAddr: r.RemoteAddr,
		UserAgent:  r.UserAgent(),
		Medias:     core.ParseQuery(r.URL.Query()),
	}

	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			if _, err := w.Write(data); err != nil && exit != nil {
				exit <- err
				exit = nil
			}
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	defer stream.RemoveConsumer(cons)

	w.Header().Set("Content-Type", cons.MimeType())

	data, err := cons.Init()
	if err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	if _, err = w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	cons.Start()

	var duration *time.Timer
	if s := r.URL.Query().Get("duration"); s != "" {
		if i, _ := strconv.Atoi(s); i > 0 {
			duration = time.AfterFunc(time.Second*time.Duration(i), func() {
				if exit != nil {
					exit <- nil
					exit = nil
				}
			})
		}
	}

	err = <-exit

	log.Trace().Err(err).Caller().Send()

	if duration != nil {
		duration.Stop()
	}
}
