package hls

import (
	"errors"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"time"
)

func handlerWSHLS(tr *ws.Transport, msg *ws.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	codecs := msg.String()

	log.Trace().Msgf("[hls] new ws consumer codecs=%s", codecs)

	cons := &mp4.Consumer{
		Desc:       "HLS/WebSocket",
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

	// bandwidth important for Safari, codecs useful for smooth playback
	data := `#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=192000,CODECS="` + cons.MimeCodecs() + `"
hls/playlist.m3u8?id=` + sid

	tr.Write(&ws.Message{Type: "hls", Value: data})

	return nil
}
