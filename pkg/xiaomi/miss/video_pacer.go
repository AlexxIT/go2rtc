package miss

import (
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const videoClockRate = 90000

type pacedVideoFrame struct {
	timestamp uint32
	receiver  *core.Receiver
	packets   []*core.Packet
}

// videoPacer absorbs the bursty delivery used by some Xiaomi CS2 cameras.
// Packets with the same RTP timestamp form one encoded frame and are released
// together. Frames are then released according to the camera RTP clock.
type videoPacer struct {
	buffer time.Duration
	frames chan pacedVideoFrame
	stop   chan struct{}
	done   chan struct{}

	latestTimestamp atomic.Uint32
	current         pacedVideoFrame
}

func newVideoPacer(buffer time.Duration) *videoPacer {
	p := &videoPacer{
		buffer: buffer,
		frames: make(chan pacedVideoFrame, 1024),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go p.run()
	return p
}

func (p *videoPacer) Write(receiver *core.Receiver, packet *core.Packet) {
	if len(p.current.packets) == 0 {
		p.current = pacedVideoFrame{
			timestamp: packet.Timestamp,
			receiver:  receiver,
			packets:   []*core.Packet{packet},
		}
		return
	}

	if p.current.timestamp == packet.Timestamp && p.current.receiver == receiver {
		p.current.packets = append(p.current.packets, packet)
		return
	}

	p.enqueueCurrent()
	p.current = pacedVideoFrame{
		timestamp: packet.Timestamp,
		receiver:  receiver,
		packets:   []*core.Packet{packet},
	}
}

func (p *videoPacer) enqueueCurrent() {
	if len(p.current.packets) == 0 {
		return
	}
	p.latestTimestamp.Store(p.current.timestamp)
	p.frames <- p.current
	p.current = pacedVideoFrame{}
}

func (p *videoPacer) Close() {
	close(p.stop)
	<-p.done
}

func (p *videoPacer) run() {
	defer close(p.done)

	// Let the source build a small jitter buffer before the first frame. The
	// goroutine producing CS2 packets continues filling p.frames meanwhile.
	if p.buffer > 0 {
		timer := time.NewTimer(p.buffer)
		select {
		case <-timer.C:
		case <-p.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		}
	}

	var lastTimestamp uint32
	var lastRelease time.Time
	var lastInterval = 100 * time.Millisecond

	for {
		var frame pacedVideoFrame
		select {
		case frame = <-p.frames:
		case <-p.stop:
			return
		}
		now := time.Now()
		if !lastRelease.IsZero() {
			interval := rtpDuration(frame.timestamp - lastTimestamp)
			if interval < 10*time.Millisecond || interval > 250*time.Millisecond {
				interval = lastInterval
			}
			lastInterval = interval

			backlog := rtpDuration(p.latestTimestamp.Load() - frame.timestamp)
			interval = pacedFrameInterval(interval, backlog, p.buffer)
			target := lastRelease.Add(interval)
			if wait := time.Until(target); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
				case <-p.stop:
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return
				}
			}
			now = time.Now()
			lastRelease = pacedReleaseAnchor(lastRelease, now, interval)
		}

		for _, packet := range frame.packets {
			frame.receiver.WriteRTP(packet)
		}
		lastTimestamp = frame.timestamp
		if lastRelease.IsZero() {
			lastRelease = now
		}
	}
}

func rtpDuration(delta uint32) time.Duration {
	// A delta larger than a minute indicates an RTP timestamp reset rather
	// than a real frame interval or queue depth.
	if delta > videoClockRate*60 {
		return 0
	}
	return time.Duration(delta) * time.Second / videoClockRate
}

func pacedFrameInterval(interval, backlog, buffer time.Duration) time.Duration {
	if buffer <= 0 {
		return interval
	}
	// Keep normal cadence around the requested buffer. If a recovered CS2
	// burst grows the queue, catch up gently instead of dumping all frames at
	// once. No encoded prediction frames are discarded.
	if backlog > buffer*5/2 {
		return interval / 2
	}
	if backlog > buffer*3/2 {
		return interval * 3 / 4
	}
	return interval
}

// pacedReleaseAnchor advances the pacing clock by the requested media interval
// instead of anchoring every frame to the timer's actual wake-up time. Timer
// overshoot is expected and would otherwise accumulate into seconds of latency
// during a long-running prebuffer. Rebase only after falling more than one
// frame behind so a real scheduler stall is not released as a packet burst.
func pacedReleaseAnchor(lastRelease, actual time.Time, interval time.Duration) time.Time {
	target := lastRelease.Add(interval)
	if actual.Sub(target) > interval {
		return actual
	}
	return target
}
