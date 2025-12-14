package xiaomi

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
	"github.com/pion/rtp"
)

const size8bit40ms = 8000 * 0.040

func (p *Producer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if err := p.client.SpeakerStart(); err != nil {
		return err
	}
	// TODO: check this!!!
	time.Sleep(time.Second)

	sender := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecPCMA:
		var buf []byte

		sender.Handler = func(pkt *rtp.Packet) {
			buf = append(buf, pkt.Payload...)
			for len(buf) >= size8bit40ms {
				_ = p.client.WriteAudio(miss.CodecPCMA, buf[:size8bit40ms])
				buf = buf[size8bit40ms:]
			}
		}
	case core.CodecOpus:
		sender.Handler = func(pkt *rtp.Packet) {
			_ = p.client.WriteAudio(miss.CodecOPUS, pkt.Payload)
		}
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}
