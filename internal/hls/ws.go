package hls

import (
	"errors"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
)

func handlerWSHLS(tr *ws.Transport, msg *ws.Message) error {
	stream := streams.GetOrPatch(tr.Request.URL.Query())
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	codecs := msg.String()
	medias := mp4.ParseCodecs(codecs, true)
	cons := mp4.NewConsumer(medias)
	cons.FormatName = "hls/fmp4"
	cons.WithRequest(tr.Request)

	log.Trace().Msgf("[hls] new ws consumer codecs=%s", codecs)

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return err
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

	main := session.Main()
	tr.Write(&ws.Message{Type: "hls", Value: string(main)})

	return nil
}
