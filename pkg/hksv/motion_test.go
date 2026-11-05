// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
)

// makeAVCC creates a fake AVCC packet with the given NAL type and total size.
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

func newTestDetector() (*MotionDetector, *mockClock, *motionRecorder) {
	rec := &motionRecorder{}
	det := NewMotionDetector(0, rec.onMotion, zerolog.Nop())
	clock := &mockClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	det.now = clock.now
	return det, clock, rec
}

// warmup feeds the detector with small P-frames to build baseline.
func warmup(det *MotionDetector, clock *mockClock, size int) {
	for i := 0; i < motionWarmupFrames; i++ {
		det.handlePacket(makePFrame(size))
		clock.advance(33 * time.Millisecond) // ~30fps
	}
}

// warmupWithBudgets performs warmup then sets test-friendly hold/cooldown budgets.
func warmupWithBudgets(det *MotionDetector, clock *mockClock, size, hold, cooldown int) {
	warmup(det, clock, size)
	det.holdBudget = hold
	det.cooldownBudget = cooldown
}

func TestMotionDetector_NoMotion(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmup(det, clock, 500)

	// feed same-size P-frames — no motion
	for i := 0; i < 100; i++ {
		det.handlePacket(makePFrame(500))
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

	last, ok := rec.lastCall()
	if !ok || !last {
		t.Fatal("expected motion detected")
	}
}

func TestMotionDetector_HoldTime(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmupWithBudgets(det, clock, 500, 30, 5)

	// trigger motion
	det.handlePacket(makePFrame(5000))

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	// send 20 non-triggered frames — still active (< holdBudget=30)
	for i := 0; i < 20; i++ {
		det.handlePacket(makePFrame(500))
	}

	if len(rec.calls) != 1 {
		t.Fatalf("expected only ON call during hold, got %v", rec.calls)
	}

	// send 15 more (total 35 > holdBudget=30) — should turn OFF
	for i := 0; i < 15; i++ {
		det.handlePacket(makePFrame(500))
	}

	last, _ := rec.lastCall()
	if last {
		t.Fatal("expected motion OFF after hold budget exhausted")
	}
}

func TestMotionDetector_Cooldown(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmupWithBudgets(det, clock, 500, 30, 5)

	// trigger and expire motion
	det.handlePacket(makePFrame(5000))
	for i := 0; i < 30; i++ {
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

	// send frames to expire cooldown (blocked trigger consumed 1 decrement)
	for i := 0; i < 5; i++ {
		det.handlePacket(makePFrame(500))
	}

	// now re-trigger should work
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

	if len(rec.calls) != 0 {
		t.Fatal("keyframes should not trigger motion")
	}

	// verify baseline didn't change
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

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	// feed large frames during motion — baseline should not change
	for i := 0; i < 50; i++ {
		det.handlePacket(makePFrame(5000))
	}

	if det.baseline != baselineBefore {
		t.Fatalf("baseline changed during motion: %f -> %f", baselineBefore, det.baseline)
	}
}

func TestMotionDetector_CustomThreshold(t *testing.T) {
	det, clock, rec := newTestDetector()
	det.threshold = 1.5

	warmup(det, clock, 500)

	// 1.6x — below default 2.0 but above custom 1.5
	det.handlePacket(makePFrame(800))

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatalf("expected motion ON with custom threshold 1.5, got %v", rec.calls)
	}
}

func TestMotionDetector_CustomThresholdNoFalsePositive(t *testing.T) {
	det, clock, rec := newTestDetector()
	det.threshold = 3.0

	warmup(det, clock, 500)

	// 2.5x — above default 2.0 but below custom 3.0
	det.handlePacket(makePFrame(1250))

	if len(rec.calls) != 0 {
		t.Fatalf("expected no motion with high threshold 3.0, got %v", rec.calls)
	}
}

func TestMotionDetector_HoldTimeExtended(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmupWithBudgets(det, clock, 500, 30, 5)

	// trigger motion
	det.handlePacket(makePFrame(5000))

	if len(rec.calls) != 1 || !rec.calls[0] {
		t.Fatal("expected motion ON")
	}

	// send 25 non-triggered frames (remainingHold 30→5)
	for i := 0; i < 25; i++ {
		det.handlePacket(makePFrame(500))
	}

	// re-trigger — remainingHold resets to 30
	det.handlePacket(makePFrame(5000))

	// send 25 more non-triggered (remainingHold 30→5)
	for i := 0; i < 25; i++ {
		det.handlePacket(makePFrame(500))
	}

	// should still be ON
	if len(rec.calls) != 1 {
		t.Fatalf("expected hold time to be extended by re-trigger, got %v", rec.calls)
	}

	// send 10 more to exhaust hold
	for i := 0; i < 10; i++ {
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

	// feed gradually larger frames — baseline should drift up
	for i := 0; i < 200; i++ {
		det.handlePacket(makePFrame(700))
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

	if len(rec.calls) != 2 {
		t.Fatalf("expected ON+OFF, got %v", rec.calls)
	}
}

func TestMotionDetector_StopWithoutMotion(t *testing.T) {
	det, clock, _ := newTestDetector()

	warmup(det, clock, 500)

	rec := &motionRecorder{}
	det.OnMotion = rec.onMotion
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

	for i := 0; i < motionWarmupFrames; i++ {
		size := 400 + (i%5)*50
		det.handlePacket(makePFrame(size))
		clock.advance(33 * time.Millisecond)
	}

	if det.baseline < 400 || det.baseline > 600 {
		t.Fatalf("baseline should be in 400-600 range, got %f", det.baseline)
	}
}

func TestMotionDetector_MultipleCycles(t *testing.T) {
	det, clock, rec := newTestDetector()

	warmupWithBudgets(det, clock, 500, 30, 5)

	for cycle := 0; cycle < 3; cycle++ {
		det.handlePacket(makePFrame(5000)) // trigger ON
		for i := 0; i < 30; i++ {         // expire hold
			det.handlePacket(makePFrame(500))
		}
		for i := 0; i < 6; i++ { // expire cooldown
			det.handlePacket(makePFrame(500))
		}
	}

	if len(rec.calls) != 6 {
		t.Fatalf("expected 6 calls (3 cycles), got %d: %v", len(rec.calls), rec.calls)
	}
	for i, v := range rec.calls {
		expected := i%2 == 0
		if v != expected {
			t.Fatalf("call[%d] = %v, expected %v", i, v, expected)
		}
	}
}

func TestMotionDetector_TriggerLevel(t *testing.T) {
	det, clock, _ := newTestDetector()

	warmup(det, clock, 500)

	expected := int(det.baseline * det.threshold)
	if det.triggerLevel != expected {
		t.Fatalf("triggerLevel = %d, expected %d", det.triggerLevel, expected)
	}
}

func TestMotionDetector_DefaultFPSCalibration(t *testing.T) {
	det, clock, _ := newTestDetector()

	warmup(det, clock, 500)

	// calibrate uses default 30fps
	expectedHold := int(motionHoldTime.Seconds() * motionDefaultFPS)
	expectedCooldown := int(motionCooldown.Seconds() * motionDefaultFPS)
	if det.holdBudget != expectedHold {
		t.Fatalf("holdBudget = %d, expected %d", det.holdBudget, expectedHold)
	}
	if det.cooldownBudget != expectedCooldown {
		t.Fatalf("cooldownBudget = %d, expected %d", det.cooldownBudget, expectedCooldown)
	}
}

func TestMotionDetector_FPSRecalibration(t *testing.T) {
	det, clock, _ := newTestDetector()

	warmup(det, clock, 500)

	// initial budgets use default 30fps
	initialHold := det.holdBudget

	// send motionTraceFrames frames with 100ms intervals → FPS=10
	for i := 0; i < motionTraceFrames; i++ {
		clock.advance(100 * time.Millisecond)
		det.handlePacket(makePFrame(500))
	}

	// after recalibration, holdBudget should reflect ~10fps (±5% due to warmup tail)
	expectedHold := int(motionHoldTime.Seconds() * 10.0) // ~300
	if det.holdBudget < expectedHold-20 || det.holdBudget > expectedHold+20 {
		t.Fatalf("holdBudget after recalibration = %d, expected ~%d (was %d)", det.holdBudget, expectedHold, initialHold)
	}
}

func BenchmarkMotionDetector_HandlePacket(b *testing.B) {
	det, clock, _ := newTestDetector()
	warmup(det, clock, 500)

	pkt := makePFrame(600)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		det.handlePacket(pkt)
	}
}

func BenchmarkMotionDetector_WithKeyframes(b *testing.B) {
	det, clock, _ := newTestDetector()
	warmup(det, clock, 500)

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
		det := NewMotionDetector(0, func(bool) {}, zerolog.Nop())
		det.now = time.Now
		for j := 0; j < motionWarmupFrames; j++ {
			det.handlePacket(pkt)
		}
	}
}
