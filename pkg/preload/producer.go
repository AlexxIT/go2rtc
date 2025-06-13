package preload

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Preload struct {
	core.Connection
	closed core.Waiter
}

func NewPreload(name string, query url.Values) *Preload {
	medias := core.ParseQuery(query)

	for _, value := range query["microphone"] {
		media := &core.Media{Kind: core.KindAudio, Direction: core.DirectionRecvonly}

		for _, name := range strings.Split(value, ",") {
			name = strings.ToUpper(name)
			switch name {
			case "", "COPY":
				name = core.CodecAny
			}
			media.Codecs = append(media.Codecs, &core.Codec{Name: name})
		}

		medias = append(medias, media)
	}

	if len(medias) == 0 {
		medias = []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{{Name: core.CodecAny}},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{{Name: core.CodecAny}},
			},
		}
	}

	return &Preload{
		Connection: core.Connection{
			ID:         core.NewID(),
			Medias:     medias,
			Protocol:   "native",
			RemoteAddr: "localhost",
			UserAgent:  "go2rtc/preload",
		},
	}
}

func (p *Preload) GetMedias() []*core.Media {
	return p.Medias
}

func (p *Preload) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Bind(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *Preload) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	receiver := core.NewReceiver(media, codec)
	p.Receivers = append(p.Receivers, receiver)
	return receiver, nil
}

func (p *Preload) Start() error {
	p.closed.Wait()
	return nil
}

func (p *Preload) Stop() error {
	for _, receiver := range p.Receivers {
		receiver.Close()
	}
	for _, sender := range p.Senders {
		sender.Close()
	}
	p.closed.Done(nil)
	return nil
}
