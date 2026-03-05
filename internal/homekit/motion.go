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
	motionDefaultFPS   = 30.0

	// recalibrate FPS and emit trace log every N frames (~5s at 30fps)
	motionTraceFrames = 150
)

type motionDetector struct {
	core.Connection
	server *server
	done   chan struct{}

	// algorithm state (accessed only from Sender goroutine — no mutex needed)
	threshold    float64
	triggerLevel int     // pre-computed: int(baseline * threshold)
	baseline     float64
	initialized  bool
	frameCount   int

	// frame-based timing (calibrated periodically, no time.Now() in per-frame hot path)
	holdBudget        int // motionHoldTime converted to frames
	cooldownBudget    int // motionCooldown converted to frames
	remainingHold     int // frames left until hold expires (active motion)
	remainingCooldown int // frames left until cooldown expires (after OFF)

	// motion state
	motionActive bool

	// periodic FPS recalibration
	lastFPSCheck time.Time
	lastFPSFrame int

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

func (m *motionDetector) calibrate() {
	// use default FPS — real FPS calibrated after first periodic check
	m.holdBudget = int(motionHoldTime.Seconds() * motionDefaultFPS)
	m.cooldownBudget = int(motionCooldown.Seconds() * motionDefaultFPS)
	m.triggerLevel = int(m.baseline * m.threshold)
	m.lastFPSCheck = m.now()
	m.lastFPSFrame = m.frameCount

	log.Debug().Str("stream", m.streamName()).
		Float64("baseline", m.baseline).
		Int("holdFrames", m.holdBudget).Int("cooldownFrames", m.cooldownBudget).
		Msg("[homekit] motion: warmup complete")
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

	size := len(payload)
	m.frameCount++

	if m.frameCount <= motionWarmupFrames {
		fsize := float64(size)
		if !m.initialized {
			m.baseline = fsize
			m.initialized = true
		} else {
			m.baseline += motionAlphaFast * (fsize - m.baseline)
		}
		if m.frameCount == motionWarmupFrames {
			m.calibrate()
		}
		return
	}

	if m.triggerLevel <= 0 {
		return
	}

	// integer comparison — no float division needed
	triggered := size > m.triggerLevel

	if !m.motionActive {
		// idle path: decrement cooldown, check for trigger, update baseline
		if m.remainingCooldown > 0 {
			m.remainingCooldown--
		}

		if triggered && m.remainingCooldown <= 0 {
			m.motionActive = true
			m.remainingHold = m.holdBudget
			log.Debug().Str("stream", m.streamName()).
				Float64("ratio", float64(size)/m.baseline).
				Msg("[homekit] motion: ON")
			m.setMotion(true)
		}

		// update baseline only if still idle (trigger frame doesn't pollute baseline)
		if !m.motionActive {
			fsize := float64(size)
			m.baseline += motionAlphaSlow * (fsize - m.baseline)
			m.triggerLevel = int(m.baseline * m.threshold)
		}
	} else {
		// active motion path: pure integer arithmetic, zero time.Now() calls
		if triggered {
			m.remainingHold = m.holdBudget
		} else {
			m.remainingHold--
			if m.remainingHold <= 0 {
				m.motionActive = false
				m.remainingCooldown = m.cooldownBudget
				log.Debug().Str("stream", m.streamName()).Msg("[homekit] motion: OFF (hold expired)")
				m.setMotion(false)
			}
		}
	}

	// periodic: recalibrate FPS and emit trace log
	if m.frameCount%motionTraceFrames == 0 {
		now := m.now()
		frames := m.frameCount - m.lastFPSFrame
		if frames > 0 {
			if elapsed := now.Sub(m.lastFPSCheck); elapsed > time.Millisecond {
				fps := float64(frames) / elapsed.Seconds()
				m.holdBudget = int(motionHoldTime.Seconds() * fps)
				m.cooldownBudget = int(motionCooldown.Seconds() * fps)
			}
		}
		m.lastFPSCheck = now
		m.lastFPSFrame = m.frameCount

		log.Trace().Str("stream", m.streamName()).
			Float64("baseline", m.baseline).Float64("ratio", float64(size)/m.baseline).
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
