package miss

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

func (p *Producer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if err := p.client.StartSpeaker(); err != nil {
		return err
	}
	// TODO: check this!!!
	time.Sleep(time.Second)

	sender := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecPCMA:
		var buf []byte

		if p.client.SpeakerCodec() == codecPCM {
			dst := &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
			transcode := pcm.Transcode(dst, track.Codec)

			sender.Handler = func(pkt *rtp.Packet) {
				buf = append(buf, transcode(pkt.Payload)...)
				const size = 2 * 8000 * 0.040 // 16bit 40ms
				for len(buf) >= size {
					p.Send += size
					_ = p.client.WriteAudio(codecPCM, buf[:size])
					buf = buf[size:]
				}
			}
		} else {
			// speaker uses the same rate as the camera audio (ex. PCMA/16000)
			rate := p.audioRate()
			var transcode func([]byte) []byte
			if track.Codec.ClockRate != rate {
				dst := &core.Codec{Name: core.CodecPCMA, ClockRate: rate}
				transcode = pcm.Transcode(dst, track.Codec)
			}
			size := int(rate) * 40 / 1000 // 8bit 40 ms

			sender.Handler = func(pkt *rtp.Packet) {
				if transcode != nil {
					buf = append(buf, transcode(pkt.Payload)...)
				} else {
					buf = append(buf, pkt.Payload...)
				}
				for len(buf) >= size {
					p.Send += size
					_ = p.client.WriteAudio(codecPCMA, buf[:size])
					buf = buf[size:]
				}
			}
		}
	case core.CodecOpus:
		if p.client.SpeakerCodec() == codecOPUS {
			var buf []byte
			sender.Handler = func(pkt *rtp.Packet) {
				if buf == nil {
					buf = pkt.Payload
				} else {
					// convert two 20ms to one 40ms
					buf = opus.JoinFrames(buf, pkt.Payload)
					p.Send += len(buf)
					_ = p.client.WriteAudio(codecOPUS, buf)
					buf = nil
				}
			}
		} else {
			sender.Handler = func(pkt *rtp.Packet) {
				p.Send += len(pkt.Payload)
				_ = p.client.WriteAudio(codecOPUS, pkt.Payload)
			}
		}
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *Producer) audioRate() uint32 {
	for _, media := range p.Medias {
		if media.Kind == core.KindAudio && media.Direction == core.DirectionRecvonly {
			return media.Codecs[0].ClockRate
		}
	}
	return 8000
}
