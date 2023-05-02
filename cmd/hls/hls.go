package hls

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog/log"
	"net/http"
	"strings"
	"sync"
	"time"
)

func Init() {
	api.HandleFunc("api/stream.m3u8", handlerStream)
	api.HandleFunc("api/hls/playlist.m3u8", handlerPlaylist)

	// HLS (TS)
	api.HandleFunc("api/hls/segment.ts", handlerSegmentTS)

	// HLS (fMP4)
	api.HandleFunc("api/hls/init.mp4", handlerInit)
	api.HandleFunc("api/hls/segment.m4s", handlerSegmentMP4)
}

type Consumer interface {
	core.Consumer
	Listen(f core.EventFunc)
	Init() ([]byte, error)
	MimeCodecs() string
	Start()
}

type Session struct {
	cons     Consumer
	playlist string
	init     []byte
	segment  []byte
	seq      int
	alive    *time.Timer
	mu       sync.Mutex
}

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
	stream := streams.GetOrNew(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	var cons Consumer

	// use fMP4 with codecs filter and TS without
	medias := mp4.ParseQuery(r.URL.Query())
	if medias != nil {
		cons = &mp4.Consumer{
			RemoteAddr: tcp.RemoteAddr(r),
			UserAgent:  r.UserAgent(),
			Medias:     medias,
		}
	} else {
		cons = &mpegts.Consumer{
			RemoteAddr: tcp.RemoteAddr(r),
			UserAgent:  r.UserAgent(),
		}
	}

	session := &Session{cons: cons}

	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			session.mu.Lock()
			session.segment = append(session.segment, data...)
			session.mu.Unlock()
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return
	}

	session.alive = time.AfterFunc(keepalive, func() {
		stream.RemoveConsumer(cons)
	})
	session.init, _ = cons.Init()

	cons.Start()

	sid := core.RandString(8, 62)

	// two segments important for Chromecast
	if medias != nil {
		session.playlist = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXT-X-MAP:URI="init.mp4?id=` + sid + `"
#EXTINF:0.500,
segment.m4s?id=` + sid + `&n=%d
#EXTINF:0.500,
segment.m4s?id=` + sid + `&n=%d`
	} else {
		session.playlist = `#EXTM3U
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

	// Apple Safari can play FLAC codec, but fail it it in m3u8 playlist
	codecs := strings.Replace(cons.MimeCodecs(), mp4.MimeFlac, mp4.MimeAAC, 1)

	// bandwidth important for Safari, codecs useful for smooth playback
	data := []byte(`#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1000000,CODECS="` + codecs + `"
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

	s := fmt.Sprintf(session.playlist, session.seq, session.seq, session.seq+1)

	if _, err := w.Write([]byte(s)); err != nil {
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

	var i byte
	for len(session.segment) == 0 {
		if i++; i > 10 {
			http.NotFound(w, r)
			return
		}
		time.Sleep(time.Millisecond * 100)
	}

	session.mu.Lock()
	data := session.segment
	// important to start new segment with init
	session.segment = session.init
	session.seq++
	session.mu.Unlock()

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

	if _, err := w.Write(session.init); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}

func handlerSegmentMP4(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Add("Content-Type", "video/iso.segment")

	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
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

	var i byte
	for len(session.segment) == 0 {
		if i++; i > 10 {
			http.NotFound(w, r)
			return
		}
		time.Sleep(time.Millisecond * 100)
	}

	session.mu.Lock()
	data := session.segment
	session.segment = nil
	session.seq++
	session.mu.Unlock()

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Caller().Send()
	}
}
