package onvif

import (
	"context"
	"net/url"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
	"github.com/pion/rtp"
)

// getProfile describes a stream from the codecs its source provides.
// A running source answers immediately; an idle source is connected for the probe,
// unless the client has already disconnected (ctx is done).
func getProfile(ctx context.Context, name string) *onvif.Profile {
	stream := streams.Get(name)
	if stream == nil || ctx.Err() != nil {
		return onvif.NewProfile(name, nil)
	}

	cons := newProber()
	if err := stream.AddConsumer(cons); err != nil {
		log.Debug().Err(err).Str("stream", name).Msg("[onvif] probe")
		return onvif.NewProfile(name, nil)
	}

	stream.RemoveConsumer(cons)

	return onvif.NewProfile(name, cons.Codecs())
}

type prober struct {
	core.Connection

	codecs []*core.Codec
}

func newProber() *prober {
	return &prober{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "onvif",
			Medias:     core.ParseQuery(url.Values{"video": {""}}),
		},
	}
}

func (p *prober) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	p.codecs = append(p.codecs, track.Codec.Clone())

	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(*rtp.Packet) {}
	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *prober) Start() error {
	return nil
}

func (p *prober) Codecs() []*core.Codec {
	return p.codecs
}
