package probe

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

type Probe struct {
	core.Connection
}

func NewProbe(query url.Values) *Probe {
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

	return &Probe{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "probe",
			Medias:     medias,
		},
	}
}

func (p *Probe) GetMedias() []*core.Media {
	return p.Medias
}

func (p *Probe) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, track.Codec)
	sender.Bind(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *Probe) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	receiver := core.NewReceiver(media, codec)
	p.Receivers = append(p.Receivers, receiver)
	return receiver, nil
}

func (p *Probe) Start() error {
	return nil
}

func (p *Probe) Stop() error {
	for _, receiver := range p.Receivers {
		receiver.Close()
	}
	for _, sender := range p.Senders {
		sender.Close()
	}
	return nil
}
