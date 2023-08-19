package hls

import (
	"net/http"
	"strings"
	"sync"
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
	log = app.GetLogger("hls")

	api.HandleFunc("api/stream.m3u8", handlerStream)
	api.HandleFunc("api/hls/playlist.m3u8", handlerPlaylist)

	// HLS (TS)
	api.HandleFunc("api/hls/segment.ts", handlerSegmentTS)

	// HLS (fMP4)
	api.HandleFunc("api/hls/init.mp4", handlerInit)
	api.HandleFunc("api/hls/segment.m4s", handlerSegmentMP4)

	ws.HandleFunc("hls", handlerWSHLS)
}

type Consumer interface {
	core.Consumer
	Listen(f core.EventFunc)
	Init() ([]byte, error)
	MimeCodecs() string
	Start()
}

var log zerolog.Logger

const keepalive = 5 * time.Second

var sessions = map[string]*Session{}

// once I saw 404 on MP4 segment, so better to use mutex
var sessionsMu sync.RWMutex

func handlerStream(w http.ResponseWriter, r *http.Request) {
	// CORS important for Chromecast
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		return
	}

	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	var cons Consumer

	// use fMP4 with codecs filter and TS without
	medias := mp4.ParseQuery(r.URL.Query())
	if medias != nil {
		cons = &mp4.Consumer{
			Desc:       "HLS/HTTP",
			RemoteAddr: tcp.RemoteAddr(r),
			UserAgent:  r.UserAgent(),
			Medias:     medias,
		}
	} else {
		//cons = &mpegts.Consumer{
		//	RemoteAddr: tcp.RemoteAddr(r),
		//	UserAgent:  r.UserAgent(),
		//}
	}

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	session := &Session{cons: cons}

	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			session.mu.Lock()
			session.buffer = append(session.buffer, data...)
			session.mu.Unlock()
		}
	})

	sid := core.RandString(8, 62)

	session.alive = time.AfterFunc(keepalive, func() {
		sessionsMu.Lock()
		delete(sessions, sid)
		sessionsMu.Unlock()

		stream.RemoveConsumer(cons)
	})
	session.init, _ = cons.Init()

	cons.Start()

	// two segments important for Chromecast
	if medias != nil {
		session.template = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXT-X-MAP:URI="init.mp4?id=` + sid + `"
#EXTINF:0.500,
segment.m4s?id=` + sid + `&n=%d
#EXTINF:0.500,
segment.m4s?id=` + sid + `&n=%d`
	} else {
		session.template = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXTINF:0.500,
segment.ts?id=` + sid + `&n=%d
#EXTINF:0.500,
segment.ts?id=` + sid + `&n=%d`
	}

	sessionsMu.Lock()
	sessions[sid] = session
	sessionsMu.Unlock()

	codecs := strings.Replace(cons.MimeCodecs(), mp4.MimeFlac, "fLaC", 1)

	// bandwidth important for Safari, codecs useful for smooth playback
	data := []byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=192000,CODECS="` + codecs + `"
hls/playlist.m3u8?id=` + sid)

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerPlaylist(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		return
	}

	sid := r.URL.Query().Get("id")
	sessionsMu.RLock()
	session := sessions[sid]
	sessionsMu.RUnlock()
	if session == nil {
		http.NotFound(w, r)
		return
	}

	if _, err := w.Write([]byte(session.Playlist())); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerSegmentTS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "video/mp2t")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		return
	}

	sid := r.URL.Query().Get("id")
	sessionsMu.RLock()
	session := sessions[sid]
	sessionsMu.RUnlock()
	if session == nil {
		http.NotFound(w, r)
		return
	}

	session.alive.Reset(keepalive)

	data := session.Segment()
	if data == nil {
		log.Warn().Msgf("[hls] can't get segment %s", r.URL.RawQuery)
		http.NotFound(w, r)
		return
	}

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerInit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-Type", "video/mp4")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		return
	}

	sid := r.URL.Query().Get("id")
	sessionsMu.RLock()
	session := sessions[sid]
	sessionsMu.RUnlock()
	if session == nil {
		http.NotFound(w, r)
		return
	}

	data := session.init
	session.init = nil

	session.segment0 = session.Segment()
	if session.segment0 == nil {
		log.Warn().Msgf("[hls] can't get init %s", r.URL.RawQuery)
		http.NotFound(w, r)
		return
	}

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerSegmentMP4(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-Type", "video/iso.segment")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET")
		return
	}

	query := r.URL.Query()

	sid := query.Get("id")
	sessionsMu.RLock()
	session := sessions[sid]
	sessionsMu.RUnlock()
	if session == nil {
		http.NotFound(w, r)
		return
	}

	session.alive.Reset(keepalive)

	var data []byte

	if query.Get("n") != "0" {
		data = session.Segment()
	} else {
		data = session.segment0
	}

	if data == nil {
		log.Warn().Msgf("[hls] can't get segment %s", r.URL.RawQuery)
		http.NotFound(w, r)
		return
	}

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}
