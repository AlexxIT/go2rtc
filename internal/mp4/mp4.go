package mp4

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("mp4")

	ws.HandleFunc("mse", handlerWSMSE)
	ws.HandleFunc("mp4", handlerWSMP4)

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

	query := r.URL.Query()
	src := query.Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := mp4.NewKeyframe(nil)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	once := &core.OnceBuffer{} // init and first frame
	_, _ = cons.WriteTo(once)

	stream.RemoveConsumer(cons)

	// Apple Safari won't show frame without length
	header := w.Header()
	header.Set("Content-Length", strconv.Itoa(once.Len()))
	header.Set("Content-Type", mp4.ContentType(cons.Codecs()))

	if filename := query.Get("filename"); filename != "" {
		header.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	}

	if _, err := once.WriteTo(w); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerMP4(w http.ResponseWriter, r *http.Request) {
	log.Trace().Msgf("[mp4] %s %+v", r.Method, r.Header)

	query := r.URL.Query()

	ua := r.UserAgent()
	if strings.Contains(ua, " Safari/") && !strings.Contains(ua, " Chrome/") && !query.Has("duration") {
		// auto redirect to HLS/fMP4 format, because Safari not support MP4 stream
		url := "stream.m3u8?" + r.URL.RawQuery
		if !query.Has("mp4") {
			url += "&mp4"
		}

		http.Redirect(w, r, url, http.StatusMovedPermanently)
		return
	}

	src := query.Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	medias := mp4.ParseQuery(r.URL.Query())
	cons := mp4.NewConsumer(medias)
	cons.Type = "MP4/HTTP active consumer"
	cons.RemoteAddr = tcp.RemoteAddr(r)
	cons.UserAgent = r.UserAgent()

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if rotate := query.Get("rotate"); rotate != "" {
		cons.Rotate = core.Atoi(rotate)
	}

	if scale := query.Get("scale"); scale != "" {
		if sx, sy, ok := strings.Cut(scale, ":"); ok {
			cons.ScaleX = core.Atoi(sx)
			cons.ScaleY = core.Atoi(sy)
		}
	}

	header := w.Header()
	header.Set("Content-Type", mp4.ContentType(cons.Codecs()))

	if filename := query.Get("filename"); filename != "" {
		header.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	}

	var duration *time.Timer
	if s := query.Get("duration"); s != "" {
		if i, _ := strconv.Atoi(s); i > 0 {
			duration = time.AfterFunc(time.Second*time.Duration(i), func() {
				_ = cons.Stop()
			})
		}
	}

	_, _ = cons.WriteTo(w)

	stream.RemoveConsumer(cons)

	if duration != nil {
		duration.Stop()
	}
}
