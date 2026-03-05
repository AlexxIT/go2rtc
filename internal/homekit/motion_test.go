package homekit

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

// makeAVCC creates a fake AVCC packet with the given NAL type and total size.
// Format: 4-byte big-endian length + NAL header + padding.
func makeAVCC(nalType byte, totalSize int) []byte {
	if totalSize < 5 {
		totalSize = 5
	}
	b := make([]byte, totalSize)
	binary.BigEndian.PutUint32(b[:4], uint32(totalSize-4))
	b[4] = nalType
	return b
}

func makePFrame(size int) *rtp.Packet {
	return &rtp.Packet{Payload: makeAVCC(h264.NALUTypePFrame, size)}
}

func makeIFrame(size int) *rtp.Packet {
	return &rtp.Packet{Payload: makeAVCC(h264.NALUTypeIFrame, size)}
}

type mockClock struct {
	t time.Time
}

func (c *mockClock) now() time.Time { return c.t }

func (c *mockClock) advance(d time.Duration) { c.t = c.t.Add(d) }

type motionRecorder struct {
	calls []bool
}

func (r *motionRecorder) onMotion(detected bool) {
	r.calls = append(r.calls, detected)
}

func (r *motionRecorder) lastCall() (bool, bool) {
	if len(r.calls) == 0 {
		return false, false
	}
	return r.calls[len(r.calls)-1], true
}

func newTestDetector() (*motionDetector, *mockClock, *motionRecorder) {
	det := newMotionDetector(nil)
	clock := &mockClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	rec := &motionRecorder{}
	det.now = clock.now
	det.onMotion = rec.onMotion
	return det, clock, rec
}

// warmup feeds the detector with small P-frames to build baseline.
func warmup(det *motionDetector, clock *mockClock, size int) {
	for i := 0; i < motionWarmupFrames; i++ {
		det.handlePacket(makePFrame(size))
		clock.advance(33 * time.Millisecond) // ~30fps
	}
}

func TestMotionDetector_NoMotion(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// feed same-size P-frames — no motion
	for i := 0; i < 100; i++ {
		det.handlePacket(makePFrame(500))
		clock.advance(33 * time.Millisecond)
	}

	if len(rec.calls) != 0 {
		t.Fatalf("expected no motion calls, got %d: %v", len(rec.calls), rec.calls)
	}
}

func TestMotionDetector_MotionDetected(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// large P-frame triggers motion
	det.handlePacket(makePFrame(5000))
	clock.advance(33 * time.Millisecond)

	last, ok := rec.lastCall()
	if !ok || !last {
		t.Fatal("expected motion detected")
	}
}

func TestMotionDetector_HoldTime(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// trigger motion
	det.handlePacket(makePFrame(5000))
	clock.advance(33 * time.Millisecond)

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	// advance 20s with small frames — still active (< holdTime)
	for i := 0; i < 60; i++ {
		clock.advance(333 * time.Millisecond)
		det.handlePacket(makePFrame(500))
	}

	// no OFF call yet
	if len(rec.calls) != 1 {
		t.Fatalf("expected only ON call during hold, got %v", rec.calls)
	}

	// advance past holdTime (30s total)
	for i := 0; i < 40; i++ {
		clock.advance(333 * time.Millisecond)
		det.handlePacket(makePFrame(500))
	}

	// now should have OFF
	last, _ := rec.lastCall()
	if last {
		t.Fatal("expected motion OFF after hold time")
	}
}

func TestMotionDetector_Cooldown(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// trigger and expire motion
	det.handlePacket(makePFrame(5000))
	clock.advance(motionHoldTime + time.Second)
	// feed enough small frames to hit a hold check interval
	for i := 0; i < motionHoldCheckFrames+1; i++ {
		det.handlePacket(makePFrame(500))
	}
	if len(rec.calls) != 2 || rec.calls[1] != false {
		t.Fatalf("expected ON then OFF, got %v", rec.calls)
	}

	// try to trigger again immediately — should be blocked by cooldown
	det.handlePacket(makePFrame(5000))
	if len(rec.calls) != 2 {
		t.Fatalf("expected cooldown to block re-trigger, got %v", rec.calls)
	}

	// advance past cooldown
	clock.advance(motionCooldown + time.Second)
	det.handlePacket(makePFrame(5000))
	if len(rec.calls) != 3 || !rec.calls[2] {
		t.Fatalf("expected motion ON after cooldown, got %v", rec.calls)
	}
}

func TestMotionDetector_SkipsKeyframes(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// huge keyframe should not trigger motion
	det.handlePacket(makeIFrame(50000))
	clock.advance(33 * time.Millisecond)

	if len(rec.calls) != 0 {
		t.Fatal("keyframes should not trigger motion")
	}

	// verify baseline didn't change by checking small P-frame doesn't trigger
	det.handlePacket(makePFrame(500))
	if len(rec.calls) != 0 {
		t.Fatal("baseline should be unaffected by keyframes")
	}
}

func TestMotionDetector_Warmup(t *testing.T) {
	det, clock, rec := newTestDetector()

	// during warmup, even large frames should not trigger
	for i := 0; i < motionWarmupFrames; i++ {
		det.handlePacket(makePFrame(5000))
		clock.advance(33 * time.Millisecond)
	}

	if len(rec.calls) != 0 {
		t.Fatal("warmup should not trigger motion")
	}
}

func TestMotionDetector_BaselineFreeze(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)
	baselineBefore := det.baseline

	// trigger motion
	det.handlePacket(makePFrame(5000))
	clock.advance(33 * time.Millisecond)

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	// feed large frames during motion — baseline should not change
	for i := 0; i < 50; i++ {
		det.handlePacket(makePFrame(5000))
		clock.advance(100 * time.Millisecond)
	}

	if det.baseline != baselineBefore {
		t.Fatalf("baseline changed during motion: %f -> %f", baselineBefore, det.baseline)
	}
}

func TestMotionDetector_CustomThreshold(t *testing.T) {
	det, clock, rec := newTestDetector()
	det.threshold = 1.5 // lower threshold

	warmup(det, clock, 500)

	// 1.6x — below default 2.0 but above custom 1.5
	det.handlePacket(makePFrame(800))
	clock.advance(33 * time.Millisecond)

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatalf("expected motion ON with custom threshold 1.5, got %v", rec.calls)
	}
}

func TestMotionDetector_CustomThresholdNoFalsePositive(t *testing.T) {
	det, clock, rec := newTestDetector()
	det.threshold = 3.0 // high threshold

	warmup(det, clock, 500)

	// 2.5x — above default 2.0 but below custom 3.0
	det.handlePacket(makePFrame(1250))
	clock.advance(33 * time.Millisecond)

	if len(rec.calls) != 0 {
		t.Fatalf("expected no motion with high threshold 3.0, got %v", rec.calls)
	}
}

func TestMotionDetector_HoldTimeExtended(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// trigger motion
	det.handlePacket(makePFrame(5000))
	clock.advance(33 * time.Millisecond)

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	// advance 25s, then re-trigger — hold timer resets
	clock.advance(25 * time.Second)
	det.handlePacket(makePFrame(5000))

	// advance another 25s (50s from first trigger, but only 25s from last)
	for i := 0; i < 75; i++ {
		clock.advance(333 * time.Millisecond)
		det.handlePacket(makePFrame(500))
	}

	// should still be ON — hold timer was reset by second trigger
	if len(rec.calls) != 1 {
		t.Fatalf("expected hold time to be extended by re-trigger, got %v", rec.calls)
	}

	// advance past hold time from last trigger
	clock.advance(6 * time.Second)
	// feed enough frames to guarantee hitting hold check interval
	for i := 0; i < motionHoldCheckFrames+1; i++ {
		det.handlePacket(makePFrame(500))
	}

	last, _ := rec.lastCall()
	if last {
		t.Fatal("expected motion OFF after extended hold expired")
	}
}

func TestMotionDetector_SmallPayloadIgnored(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// payloads < 5 bytes should be silently ignored
	det.handlePacket(&rtp.Packet{Payload: []byte{1, 2, 3, 4}})
	det.handlePacket(&rtp.Packet{Payload: nil})
	det.handlePacket(&rtp.Packet{Payload: []byte{}})

	if len(rec.calls) != 0 {
		t.Fatalf("small payloads should be ignored, got %v", rec.calls)
	}
}

func TestMotionDetector_BaselineAdapts(t *testing.T) {
	det, clock, _ := newTestDetector()

	warmup(det, clock, 500)
	baselineAfterWarmup := det.baseline

	// feed gradually larger frames (no motion active) — baseline should drift up
	for i := 0; i < 200; i++ {
		det.handlePacket(makePFrame(700))
		clock.advance(33 * time.Millisecond)
	}

	if det.baseline <= baselineAfterWarmup {
		t.Fatalf("baseline should adapt upward: before=%f after=%f", baselineAfterWarmup, det.baseline)
	}
}

func TestMotionDetector_DoubleStopSafe(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)
	det.handlePacket(makePFrame(5000))

	_ = det.Stop()
	_ = det.Stop() // second stop should not panic

	if len(rec.calls) != 2 { // ON + OFF from first Stop
		t.Fatalf("expected ON+OFF, got %v", rec.calls)
	}
}

func TestMotionDetector_StopWithoutMotion(t *testing.T) {
	det, clock, _ := newTestDetector()

	warmup(det, clock, 500)

	// stop without ever triggering motion — should not call onMotion
	rec := &motionRecorder{}
	det.onMotion = rec.onMotion
	_ = det.Stop()

	if len(rec.calls) != 0 {
		t.Fatalf("stop without motion should not call onMotion, got %v", rec.calls)
	}
}

func TestMotionDetector_StopClearsMotion(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	det.handlePacket(makePFrame(5000))
	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	_ = det.Stop()

	if len(rec.calls) != 2 || rec.calls[1] != false {
		t.Fatalf("expected Stop to clear motion, got %v", rec.calls)
	}
}

func TestMotionDetector_WarmupBaseline(t *testing.T) {
	det, clock, _ := newTestDetector()

	// feed varying sizes during warmup
	for i := 0; i < motionWarmupFrames; i++ {
		size := 400 + (i%5)*50 // 400-600 range
		det.handlePacket(makePFrame(size))
		clock.advance(33 * time.Millisecond)
	}

	// baseline should be a reasonable average, not zero or the last value
	if det.baseline < 400 || det.baseline > 600 {
		t.Fatalf("baseline should be in 400-600 range after varied warmup, got %f", det.baseline)
	}
}

func TestMotionDetector_MultipleCycles(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// 3 full motion cycles: ON → hold → OFF → cooldown → ON ...
	for cycle := 0; cycle < 3; cycle++ {
		det.handlePacket(makePFrame(5000))
		clock.advance(motionHoldTime + time.Second)
		// feed enough frames to hit hold check interval
		for i := 0; i < motionHoldCheckFrames+1; i++ {
			det.handlePacket(makePFrame(500))
		}
		clock.advance(motionCooldown + time.Second)
	}

	// expect 3 ON + 3 OFF = 6 calls
	if len(rec.calls) != 6 {
		t.Fatalf("expected 6 calls (3 cycles), got %d: %v", len(rec.calls), rec.calls)
	}
	for i, v := range rec.calls {
		expected := i%2 == 0 // ON at 0,2,4; OFF at 1,3,5
		if v != expected {
			t.Fatalf("call[%d] = %v, expected %v", i, v, expected)
		}
	}
}

func BenchmarkMotionDetector_HandlePacket(b *testing.B) {
	det, _, _ := newTestDetector()
	warmup(det, &mockClock{t: time.Now()}, 500)
	det.now = time.Now

	pkt := makePFrame(600)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		det.handlePacket(pkt)
	}
}

func BenchmarkMotionDetector_WithKeyframes(b *testing.B) {
	det, _, _ := newTestDetector()
	warmup(det, &mockClock{t: time.Now()}, 500)
	det.now = time.Now

	pFrame := makePFrame(600)
	iFrame := makeIFrame(10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%30 == 0 {
			det.handlePacket(iFrame)
		} else {
			det.handlePacket(pFrame)
		}
	}
}

func BenchmarkMotionDetector_MotionActive(b *testing.B) {
	det, clock, _ := newTestDetector()
	warmup(det, clock, 500)
	det.now = time.Now

	// trigger motion and keep it active
	det.handlePacket(makePFrame(5000))
	pkt := makePFrame(5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		det.handlePacket(pkt)
	}
}

func BenchmarkMotionDetector_Warmup(b *testing.B) {
	pkt := makePFrame(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		det := newMotionDetector(nil)
		det.onMotion = func(bool) {}
		det.now = time.Now
		for j := 0; j < motionWarmupFrames; j++ {
			det.handlePacket(pkt)
		}
	}
}
