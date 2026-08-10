package reolink

import (
	"context"
	"time"
)

func (p *Profile) watchTracks(ctx context.Context, cancel context.CancelFunc, stalled chan<- error) {
	if p.trackCheckInterval <= 0 || p.trackStallTimeout <= 0 {
		return
	}
	rawOnly := p.video == nil && p.audio == nil
	ticker := time.NewTicker(p.trackCheckInterval)
	defer ticker.Stop()
	video, audio := p.videoFrames.Load(), p.audioSamples.Load()
	videoAt, audioAt := time.Now(), time.Now()
	received, receivedAt := p.recvBytes.Load(), time.Now()
	rateSamples, rateAt := p.audioSamples.Load(), time.Now()
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
			} else if p.audio != nil && p.pipeline.audioRate > 0 && p.trackRateWindow > 0 &&
				now.Sub(rateAt) >= p.trackRateWindow {
				samples := p.audioSamples.Load()
				if audioRateLow(samples-rateSamples, p.pipeline.audioRate, now.Sub(rateAt)) {
					err = mediaErrorf("reolink: audio track rate below expected")
				} else {
					rateSamples, rateAt = samples, now
				}
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
