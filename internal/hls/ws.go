package hls

import (
	"errors"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog/log"
	"strings"
	"time"
)

func handlerWSHLS(tr *ws.Transport, msg *ws.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	codecs := msg.String()

	cons := &mp4.Consumer{
		RemoteAddr: tcp.RemoteAddr(tr.Request),
		UserAgent:  tr.Request.UserAgent(),
		Medias:     mp4.ParseCodecs(codecs, true),
	}

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	session := &Session{cons: cons}

	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			session.mu.Lock()
			session.buffer = append(session.buffer, data...)
			session.mu.Unlock()
		}
	})

	session.alive = time.AfterFunc(keepalive, func() {
		stream.RemoveConsumer(cons)
	})
	session.init, _ = cons.Init()

	cons.Start()

	sid := core.RandString(8, 62)

	// two segments important for Chromecast
	session.template = `#EXTM3U
#EXT-X-VERSION:6
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:%d
#EXT-X-MAP:URI="init.mp4?id=` + sid + `"
#EXTINF:0.500,
segment.m4s?id=` + sid + `&n=%d
#EXTINF:0.500,
segment.m4s?id=` + sid + `&n=%d`

	sessionsMu.Lock()
	sessions[sid] = session
	sessionsMu.Unlock()

	// Apple Safari can play FLAC codec, but fail it it in m3u8 playlist
	codecs = strings.Replace(cons.MimeCodecs(), mp4.MimeFlac, mp4.MimeAAC, 1)

	// bandwidth important for Safari, codecs useful for smooth playback
	data := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=1000000,CODECS="` + codecs + `"
hls/playlist.m3u8?id=` + sid

	tr.Write(&ws.Message{Type: "hls", Value: data})

	return nil
}
