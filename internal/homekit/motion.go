package homekit

import (
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

const (
	motionWarmupFrames = 30
	motionThreshold    = 2.0
	motionAlphaFast    = 0.1
	motionAlphaSlow    = 0.02
	motionHoldTime     = 30 * time.Second
	motionCooldown     = 5 * time.Second

	// check hold time expiry every N frames during active motion (~270ms at 30fps)
	motionHoldCheckFrames = 8
	// trace log every N frames (~5s at 30fps)
	motionTraceFrames = 150
)

type motionDetector struct {
	core.Connection
	server *server
	done   chan struct{}

	// algorithm state (accessed only from Sender goroutine — no mutex needed)
	threshold   float64
	baseline    float64
	initialized bool
	frameCount  int

	// motion state
	motionActive bool
	lastMotion   time.Time
	lastOff      time.Time

	// for testing: injectable time and callback
	now      func() time.Time
	onMotion func(bool)
}

func newMotionDetector(srv *server) *motionDetector {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
			},
		},
	}
	threshold := motionThreshold
	if srv != nil && srv.motionThreshold > 0 {
		threshold = srv.motionThreshold
	}
	return &motionDetector{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "motion",
			Protocol:   "detect",
			Medias:     medias,
		},
		server:    srv,
		threshold: threshold,
		done:      make(chan struct{}),
		now:       time.Now,
	}
}

func (m *motionDetector) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	log.Debug().Str("stream", m.streamName()).Str("codec", track.Codec.Name).Msg("[homekit] motion: add track")

	codec := track.Codec.Clone()
	sender := core.NewSender(media, codec)

	sender.Handler = func(packet *rtp.Packet) {
		m.handlePacket(packet)
	}

	if track.Codec.IsRTP() {
		sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
	} else {
		sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
	}

	sender.HandleRTP(track)
	m.Senders = append(m.Senders, sender)
	return nil
}

func (m *motionDetector) streamName() string {
	if m.server != nil {
		return m.server.stream
	}
	return ""
}

func (m *motionDetector) handlePacket(packet *rtp.Packet) {
	payload := packet.Payload
	if len(payload) < 5 {
		return
	}

	// skip keyframes — always large, not informative for motion
	if h264.IsKeyframe(payload) {
		return
	}

	size := float64(len(payload))
	m.frameCount++

	if m.frameCount <= motionWarmupFrames {
		// warmup: build baseline with fast EMA
		if !m.initialized {
			m.baseline = size
			m.initialized = true
		} else {
			m.baseline += motionAlphaFast * (size - m.baseline)
		}
		if m.frameCount == motionWarmupFrames {
			log.Debug().Str("stream", m.streamName()).Float64("baseline", m.baseline).Msg("[homekit] motion: warmup complete")
		}
		return
	}

	if m.baseline <= 0 {
		return
	}

	ratio := size / m.baseline
	triggered := ratio > m.threshold

	if !m.motionActive {
		// idle path: check for trigger first, then update baseline
		if triggered {
			// only call time.Now() when threshold exceeded
			now := m.now()
			if now.Sub(m.lastOff) >= motionCooldown {
				m.motionActive = true
				m.lastMotion = now
				log.Debug().Str("stream", m.streamName()).Float64("ratio", ratio).Msg("[homekit] motion: ON")
				m.setMotion(true)
			} else {
				log.Debug().Str("stream", m.streamName()).Float64("ratio", ratio).
					Dur("cooldown_left", motionCooldown-now.Sub(m.lastOff)).Msg("[homekit] motion: blocked by cooldown")
			}
		}
		// update baseline only if still idle (trigger frame doesn't pollute baseline)
		if !m.motionActive {
			m.baseline += motionAlphaSlow * (size - m.baseline)
		}
	} else {
		// active motion path
		if triggered {
			m.lastMotion = m.now()
		} else if m.frameCount%motionHoldCheckFrames == 0 {
			// check hold time expiry periodically, not every frame
			now := m.now()
			if now.Sub(m.lastMotion) >= motionHoldTime {
				m.motionActive = false
				m.lastOff = now
				log.Debug().Str("stream", m.streamName()).Msg("[homekit] motion: OFF (hold expired)")
				m.setMotion(false)
			}
		}
	}

	// periodic trace using frame counter instead of time check
	if m.frameCount%motionTraceFrames == 0 {
		log.Trace().Str("stream", m.streamName()).
			Float64("baseline", m.baseline).Float64("ratio", ratio).
			Bool("active", m.motionActive).Msg("[homekit] motion: status")
	}
}

func (m *motionDetector) setMotion(detected bool) {
	if m.onMotion != nil {
		m.onMotion(detected)
		return
	}
	if m.server != nil {
		m.server.SetMotionDetected(detected)
	}
}

func (m *motionDetector) WriteTo(io.Writer) (int64, error) {
	<-m.done
	return 0, nil
}

func (m *motionDetector) Stop() error {
	select {
	case <-m.done:
	default:
		if m.motionActive {
			m.motionActive = false
			log.Debug().Str("stream", m.streamName()).Msg("[homekit] motion: OFF (stop)")
			m.setMotion(false)
		}
		close(m.done)
	}
	return m.Connection.Stop()
}
