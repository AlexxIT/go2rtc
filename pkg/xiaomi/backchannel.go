package xiaomi

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
	"github.com/pion/rtp"
)

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

		if p.model == "isa.camera.hlc6" {
			dst := &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
			transcode := pcm.Transcode(dst, track.Codec)

			sender.Handler = func(pkt *rtp.Packet) {
				buf = append(buf, transcode(pkt.Payload)...)
				const size = 2 * 8000 * 0.040 // 16bit 40ms
				for len(buf) >= size {
					_ = p.client.WriteAudio(miss.CodecPCM, buf[:size])
					buf = buf[size:]
				}
			}
		} else {
			sender.Handler = func(pkt *rtp.Packet) {
				buf = append(buf, pkt.Payload...)
				const size = 8000 * 0.040 // 8bit 40 ms
				for len(buf) >= size {
					_ = p.client.WriteAudio(miss.CodecPCMA, buf[:size])
					buf = buf[size:]
				}
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
