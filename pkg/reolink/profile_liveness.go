package reolink

import (
	"context"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/baichuan"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
)

var errAudioRateBelowExpected = mediaErrorf("reolink: audio track rate below expected")

func (p *Producer) writePacket(packet baichuan.MediaPacket) error {
	frame, err := p.pipeline.frame(packet)
	if frame.valid() {
		p.writeFrame(frame)
	}
	if err != nil {
		return err
	}
	if !frame.valid() && (packet.Kind == baichuan.MediaVideoI || packet.Kind == baichuan.MediaVideoP) {
		p.invalidateVideoRate()
	}
	if frame.malformed {
		return p.recoverAudio(packet, frame)
	}
	return nil
}

func (p *Producer) recoverAudio(packet baichuan.MediaPacket, frame mediaFrame) error {
	if frame.malformed && p.audio != nil {
		if frame.converted {
			// Restore only our owned, in-place conversion for protocol recovery.
			annexb.DecodeAVCC(packet.Data, false)
		}
		return packet.RecoverAudio(func(packet baichuan.MediaPacket) error {
			if packet.Kind != p.pipeline.audioKind {
				return nil
			}
			if packet.Kind == baichuan.MediaAAC {
				if config, ok := aacConfig(packet.Data); !ok || config != p.pipeline.audioConfig {
					return nil
				}
			}
			frame, err := p.pipeline.audioFrame(packet)
			if err == nil && frame.valid() {
				p.writeFrame(frame)
			}
			return err
		})
	}
	return nil
}

func (p *Producer) invalidateVideoRate() {
	p.videoRateEpoch |= 1
	if p.video != nil && p.audio != nil {
		p.videoRate.Store(uint64(p.videoRateEpoch)<<32 | uint64(uint32(p.pipeline.videoTS.value)))
	}
}

type audioRateWindow struct {
	at       time.Time
	samples  uint64
	video    uint64
	videoSet bool
}

func (p *Producer) checkAudioRate(now time.Time, video uint64, window *audioRateWindow) bool {
	if p.pipeline.audioRate == 0 || p.trackRateWindow <= 0 {
		return false
	}
	clock := window.video
	if p.video != nil {
		clock = p.videoRate.Load()
		valid := clock>>32&1 == 0
		if !valid || !window.videoSet || clock>>32 != window.video>>32 {
			*window = audioRateWindow{at: now, samples: p.audioSamples.Load(), video: clock, videoSet: valid && video != 0}
			return false
		}
	}
	elapsed := now.Sub(window.at)
	if elapsed < p.trackRateWindow {
		return false
	}
	if p.video != nil {
		// A shared backlog slows both tracks; accepted camera skips still
		// count as missing scene time when audio does not cover them.
		span := time.Duration(uint32(clock)-uint32(window.video)) * time.Second / 90000
		if span < elapsed {
			elapsed = span
		}
	}
	samples := p.audioSamples.Load()
	low := audioRateLow(samples-window.samples, p.pipeline.audioRate, elapsed)
	*window = audioRateWindow{at: now, samples: samples, video: clock, videoSet: video != 0}
	return low
}

func (p *Producer) watchTracks(ctx context.Context, cancel context.CancelFunc, stalled chan<- error) {
	if p.trackCheckInterval <= 0 || p.trackStallTimeout <= 0 {
		return
	}
	rawOnly := p.video == nil && p.audio == nil
	ticker := time.NewTicker(p.trackCheckInterval)
	defer ticker.Stop()
	video, audio := p.videoFrames.Load(), p.audioSamples.Load()
	videoAt, audioAt := time.Now(), time.Now()
	received, receivedAt := p.recvBytes.Load(), time.Now()
	rate := audioRateWindow{at: time.Now(), samples: audio, video: p.videoRate.Load(), videoSet: video != 0}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if frames := p.videoFrames.Load(); frames != video {
				video, videoAt = frames, now
			}
			if samples := p.audioSamples.Load(); samples != audio {
				audio, audioAt = samples, now
			}
			if rawOnly {
				if bytes := p.recvBytes.Load(); bytes != received {
					received, receivedAt = bytes, now
				}
			}
			var err error
			if rawOnly && now.Sub(receivedAt) >= p.trackStallTimeout {
				err = mediaErrorf("reolink: media stream stalled")
			} else if p.video != nil && now.Sub(videoAt) >= p.trackStallTimeout {
				err = mediaErrorf("reolink: video track stalled")
			} else if p.audio != nil && now.Sub(audioAt) >= p.trackStallTimeout {
				err = mediaErrorf("reolink: audio track stalled")
			} else if p.audio != nil && p.checkAudioRate(now, video, &rate) {
				err = errAudioRateBelowExpected
			}
			if err != nil {
				stalled <- err
				cancel()
				return
			}
		}
	}
}

func audioRateLow(samples uint64, rate uint32, elapsed time.Duration) bool {
	expected := uint64(rate) * uint64(elapsed) / uint64(time.Second)
	return samples < expected-expected/4
}
