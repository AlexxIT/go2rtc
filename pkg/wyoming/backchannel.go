package wyoming

import (
	"fmt"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Backchannel struct {
	core.Connection
	api *API
}

func newBackchannel(conn net.Conn) *Backchannel {
	return &Backchannel{
		core.Connection{
			ID:         core.NewID(),
			FormatName: "wyoming",
			Medias: []*core.Media{
				{
					Kind:      core.KindAudio,
					Direction: core.DirectionSendonly,
					Codecs: []*core.Codec{
						{Name: core.CodecPCML, ClockRate: 22050},
					},
				},
			},
			Transport: conn,
		},
		NewAPI(conn),
	}
}

func (b *Backchannel) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return nil, core.ErrCantGetTrack
}

func (b *Backchannel) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	sender := core.NewSender(media, codec)
	sender.Handler = func(pkt *rtp.Packet) {
		ts := time.Now().Nanosecond()
		evt := &Event{
			Type:    "audio-chunk",
			Data:    fmt.Sprintf(`{"rate":22050,"width":2,"channels":1,"timestamp":%d}`, ts),
			Payload: pkt.Payload,
		}
		_ = b.api.WriteEvent(evt)
	}
	sender.HandleRTP(track)
	b.Senders = append(b.Senders, sender)
	return nil
}

func (b *Backchannel) Start() error {
	for {
		if _, err := b.api.ReadEvent(); err != nil {
			return err
		}
	}
}
