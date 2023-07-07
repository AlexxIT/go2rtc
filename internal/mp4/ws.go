package mp4

import (
	"errors"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

func handlerWSMSE(tr *ws.Transport, msg *ws.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := &mp4.Consumer{
		RemoteAddr: tcp.RemoteAddr(tr.Request),
		UserAgent:  tr.Request.UserAgent(),
	}

	if codecs := msg.String(); codecs != "" {
		log.Trace().Str("codecs", codecs).Msgf("[mp4] new WS/MSE consumer")
		cons.Medias = mp4.ParseCodecs(codecs, true)
	}

	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			tr.Write(data)
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Debug().Err(err).Msg("[mp4] add consumer")
		return err
	}

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	tr.Write(&ws.Message{Type: "mse", Value: cons.MimeType()})

	data, err := cons.Init()
	if err != nil {
		log.Warn().Err(err).Caller().Send()
		return err
	}

	tr.Write(data)

	cons.Start()

	return nil
}

func handlerWSMP4(tr *ws.Transport, msg *ws.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := &mp4.Segment{
		RemoteAddr:   tcp.RemoteAddr(tr.Request),
		UserAgent:    tr.Request.UserAgent(),
		OnlyKeyframe: true,
	}

	if codecs := msg.String(); codecs != "" {
		log.Trace().Str("codecs", codecs).Msgf("[mp4] new WS/MP4 consumer")
		cons.Medias = mp4.ParseCodecs(codecs, false)
	}

	cons.Listen(func(msg any) {
		if data, ok := msg.([]byte); ok {
			tr.Write(data)
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		return err
	}

	tr.Write(&ws.Message{Type: "mp4", Value: cons.MimeType})

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}
