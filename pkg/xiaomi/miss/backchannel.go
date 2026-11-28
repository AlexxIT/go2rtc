package miss

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

const talkIdle = 3 * time.Second

func (p *Producer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if err := p.startSpeaker(); err != nil {
		return err
	}
	p.watchOnce.Do(func() { go p.speakerWatch(talkIdle) })
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
			sender.Handler = func(pkt *rtp.Packet) {
				buf = append(buf, pkt.Payload...)
				const size = 8000 * 0.040 // 8bit 40 ms
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

	if handler := sender.Handler; handler != nil {
		sender.Handler = func(pkt *rtp.Packet) {
			p.ensureSpeaker()
			handler(pkt)
		}
	}

	sender.HandleRTP(track)
	p.Senders = append(p.Senders, sender)
	return nil
}

func (p *Producer) startSpeaker() error {
	p.speakerMu.Lock()
	defer p.speakerMu.Unlock()

	p.lastTalk.Store(time.Now().UnixNano())
	if p.speakerOn.Load() {
		return nil
	}
	if err := p.client.StartSpeaker(); err != nil {
		return err
	}
	p.speakerOn.Store(true)
	return nil
}

func (p *Producer) stopSpeaker() {
	p.speakerMu.Lock()
	defer p.speakerMu.Unlock()

	if p.speakerOn.Swap(false) {
		_ = p.client.StopSpeaker()
	}
}

func (p *Producer) ensureSpeaker() {
	p.lastTalk.Store(time.Now().UnixNano())
	if p.speakerOn.Load() {
		return
	}
	_ = p.startSpeaker()
}

func (p *Producer) speakerWatch(idleTimeout time.Duration) {
	if p.done == nil {
		return
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.checkSpeakerIdle(idleTimeout)
		}
	}
}

func (p *Producer) checkSpeakerIdle(idleTimeout time.Duration) {
	p.speakerMu.Lock()
	defer p.speakerMu.Unlock()

	if !p.speakerOn.Load() {
		return
	}
	if time.Since(time.Unix(0, p.lastTalk.Load())) > idleTimeout {
		p.speakerOn.Store(false)
		_ = p.client.StopSpeaker()
	}
}
