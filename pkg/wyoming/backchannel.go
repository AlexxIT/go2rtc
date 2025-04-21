package wyoming

import (
	"fmt"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Backchannel struct {
	core.Connection
	api *API
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
			Data:    []byte(fmt.Sprintf(`{"rate":16000,"width":2,"channels":1,"timestamp":%d}`, ts)),
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
