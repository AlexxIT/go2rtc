package legacy

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

func (p *Producer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	tutk.DebugLog("legacy: AddTrack backchannel requested for model %s", p.client.model)

	if err := p.client.StartSpeaker(); err != nil {
		tutk.DebugLog("legacy backchannel StartSpeaker err: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	sender := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecPCMA:
		var buf []byte
		sender.Handler = func(pkt *rtp.Packet) {
			buf = append(buf, pkt.Payload...)
			const size = 8000 * 0.040 // 8bit 40ms = 320 bytes
			for len(buf) >= size {
				p.Send += size
				_ = p.client.WriteAudio(tutk.CodecPCMA, buf[:size])
				buf = buf[size:]
			}
		}

	case core.CodecPCMU:
		var buf []byte
		dst := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
		transcode := pcm.Transcode(dst, track.Codec)
		sender.Handler = func(pkt *rtp.Packet) {
			buf = append(buf, transcode(pkt.Payload)...)
			const size = 8000 * 0.040 // 320 bytes
			for len(buf) >= size {
				p.Send += size
				_ = p.client.WriteAudio(tutk.CodecPCMA, buf[:size])
				buf = buf[size:]
			}
		}

	case core.CodecOpus:
		var buf []byte
		dst := &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
		transcode := pcm.Transcode(dst, track.Codec)
		sender.Handler = func(pkt *rtp.Packet) {
			if transcode != nil {
				buf = append(buf, transcode(pkt.Payload)...)
				const size = 8000 * 0.040 // 320 bytes
				for len(buf) >= size {
					p.Send += size
					_ = p.client.WriteAudio(tutk.CodecPCMA, buf[:size])
					buf = buf[size:]
				}
			}
		}
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}
