package nest

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fastClamp lets the tests use millisecond stream lifetimes.
func fastClamp(t *testing.T) {
	t.Helper()
	old := minExtendDelay
	minExtendDelay = time.Millisecond
	t.Cleanup(func() { minExtendDelay = old })
}

// A session must keep extending itself for as long as Google keeps granting
// extensions, not just once.
func TestSessionExtendsRepeatedly(t *testing.T) {
	fastClamp(t)
	var calls atomic.Int32
	const life = 40 * time.Millisecond
	const lead = 10 * time.Millisecond

	s := newSession(time.Now().Add(life), lead, 0, func() (time.Time, error) {
		calls.Add(1)
		return time.Now().Add(life), nil
	})
	defer s.Stop()

	time.Sleep(3*life + lead)

	if n := calls.Load(); n < 3 {
		t.Fatalf("expected at least 3 extensions, got %d", n)
	}
}

// Once an extension fails the stream is going to die at its expiry; nothing
// further should be attempted. The producer reconnect handles the rest.
func TestSessionStopsAfterFailedExtend(t *testing.T) {
	fastClamp(t)
	var calls atomic.Int32
	const life = 40 * time.Millisecond

	s := newSession(time.Now().Add(life), 10*time.Millisecond, 0, func() (time.Time, error) {
		calls.Add(1)
		return time.Time{}, errors.New("nest: wrong status: 404 Not Found")
	})
	defer s.Stop()

	time.Sleep(4 * life)

	if n := calls.Load(); n != 1 {
		t.Fatalf("expected exactly 1 extension attempt, got %d", n)
	}
}

// Stop must cancel a pending extension.
func TestSessionStopCancelsTimer(t *testing.T) {
	fastClamp(t)
	var calls atomic.Int32
	const life = 40 * time.Millisecond

	s := newSession(time.Now().Add(life), 10*time.Millisecond, 0, func() (time.Time, error) {
		calls.Add(1)
		return time.Now().Add(life), nil
	})
	s.Stop()

	time.Sleep(2 * life)

	if n := calls.Load(); n != 0 {
		t.Fatalf("expected no extensions after Stop, got %d", n)
	}
}

// An expiry that is already inside the lead window must not spin: the first
// extension is delayed by at least minExtendDelay.
func TestSessionNeverSchedulesInstantly(t *testing.T) {
	var calls atomic.Int32

	s := newSession(time.Now(), time.Minute, 0, func() (time.Time, error) {
		calls.Add(1)
		return time.Now(), nil
	})
	defer s.Stop()

	time.Sleep(minExtendDelay / 2)

	if n := calls.Load(); n != 0 {
		t.Fatalf("extension fired before minExtendDelay, calls=%d", n)
	}
}

// A spread pulls only the first extension forward; later ones follow the
// expiry Google hands back.
func TestSessionSpreadsFirstExtend(t *testing.T) {
	fastClamp(t)
	const life = 60 * time.Millisecond
	const lead = 10 * time.Millisecond
	const spread = 30 * time.Millisecond

	start := time.Now()
	var first, second time.Time
	done := make(chan struct{})
	s := newSession(start.Add(life), lead, spread, func() (time.Time, error) {
		switch {
		case first.IsZero():
			first = time.Now()
		case second.IsZero():
			second = time.Now()
			close(done)
		}
		return time.Now().Add(life), nil
	})
	defer s.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("extensions did not happen")
	}
	if got := first.Sub(start); got < life-lead-spread-5*time.Millisecond || got > life-lead-spread+15*time.Millisecond {
		t.Fatalf("first extend after %s, expected about %s", got, life-lead-spread)
	}
	if got := second.Sub(first); got < life-lead-5*time.Millisecond || got > life-lead+15*time.Millisecond {
		t.Fatalf("second extend %s after the first, expected about %s", got, life-lead)
	}
}

func TestExtendSpreadIsDeterministicAndBounded(t *testing.T) {
	a, b := extendSpread("AVPHwEv-one"), extendSpread("AVPHwEv-one")
	if a != b {
		t.Fatal("spread must be deterministic per device")
	}
	for _, id := range []string{"a", "b", "c", "AVPHwEuTOT0ETJPmatKoIjTrR1NeceJh9mVGGo1H"} {
		if d := extendSpread(id); d < 0 || d >= 2*time.Minute {
			t.Fatalf("spread %s out of range for %q", d, id)
		}
	}
}
