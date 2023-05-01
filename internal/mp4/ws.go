package mp4

import (
	"errors"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"strings"
)

func handlerWSMSE(tr *api.Transport, msg *api.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := &mp4.Consumer{
		RemoteAddr: tcp.RemoteAddr(tr.Request),
		UserAgent:  tr.Request.UserAgent(),
	}

	if codecs := msg.String(); codecs != "" {
		log.Trace().Str("codecs", codecs).Msgf("[mp4] new WS/MSE consumer")
		cons.Medias = parseMedias(codecs, true)
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

	tr.Write(&api.Message{Type: "mse", Value: cons.MimeType()})

	data, err := cons.Init()
	if err != nil {
		log.Warn().Err(err).Caller().Send()
		return err
	}

	tr.Write(data)

	cons.Start()

	return nil
}

func handlerWSMP4(tr *api.Transport, msg *api.Message) error {
	src := tr.Request.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
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
		cons.Medias = parseMedias(codecs, false)
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

	tr.Write(&api.Message{Type: "mp4", Value: cons.MimeType})

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}

func parseMedias(codecs string, parseAudio bool) (medias []*core.Media) {
	var videos []*core.Codec
	var audios []*core.Codec

	for _, name := range strings.Split(codecs, ",") {
		switch name {
		case mp4.MimeH264:
			codec := &core.Codec{Name: core.CodecH264}
			videos = append(videos, codec)
		case mp4.MimeH265:
			codec := &core.Codec{Name: core.CodecH265}
			videos = append(videos, codec)
		case mp4.MimeAAC:
			codec := &core.Codec{Name: core.CodecAAC}
			audios = append(audios, codec)
		case mp4.MimeFlac:
			audios = append(audios,
				&core.Codec{Name: core.CodecPCMA},
				&core.Codec{Name: core.CodecPCMU},
				&core.Codec{Name: core.CodecPCM},
			)
		case mp4.MimeOpus:
			codec := &core.Codec{Name: core.CodecOpus}
			audios = append(audios, codec)
		}
	}

	if videos != nil {
		media := &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs:    videos,
		}
		medias = append(medias, media)
	}

	if audios != nil && parseAudio {
		media := &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    audios,
		}
		medias = append(medias, media)
	}

	return
}
