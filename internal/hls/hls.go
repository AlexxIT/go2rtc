package hls

import (
	"net/http"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
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

var log zerolog.Logger

const keepalive = 5 * time.Second

// once I saw 404 on MP4 segment, so better to use mutex
var sessions = map[string]*Session{}
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

	var cons core.Consumer

	// use fMP4 with codecs filter and TS without
	medias := mp4.ParseQuery(r.URL.Query())
	if medias != nil {
		c := mp4.NewConsumer(medias)
		c.FormatName = "hls/fmp4"
		c.WithRequest(r)
		cons = c
	} else {
		c := mpegts.NewConsumer()
		c.FormatName = "hls/mpegts"
		c.WithRequest(r)
		cons = c
	}

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	session := NewSession(cons)
	session.alive = time.AfterFunc(keepalive, func() {
		sessionsMu.Lock()
		delete(sessions, session.id)
		sessionsMu.Unlock()

		stream.RemoveConsumer(cons)
	})

	sessionsMu.Lock()
	sessions[session.id] = session
	sessionsMu.Unlock()

	go session.Run()

	if _, err := w.Write(session.Main()); err != nil {
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

	if _, err := w.Write(session.Playlist()); err != nil {
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

	data := session.Init()
	if data == nil {
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
