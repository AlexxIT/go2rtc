package nest

import (
	"errors"
	"testing"
	"time"
)

// testLimiter returns a limiter with a controllable clock. sleep advances the
// clock instead of blocking, and records what was slept.
func testLimiter(start time.Time) (*limiter, *time.Time, *[]time.Duration) {
	now := start
	var slept []time.Duration
	l := newLimiter()
	l.now = func() time.Time { return now }
	l.sleep = func(d time.Duration) {
		slept = append(slept, d)
		now = now.Add(d)
	}
	return l, &now, &slept
}

func TestCommandProjectBudget(t *testing.T) {
	l, now, slept := testLimiter(time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))

	// ten calls in the same minute go straight through, spread over devices
	for i := 0; i < executeCommandPerMinute; i++ {
		dev := string(rune('a' + i%4))
		if err := l.Command("p", dev, "generate", dialMaxWait); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if len(*slept) != 0 {
		t.Fatalf("expected no sleeps, got %v", *slept)
	}

	// the eleventh would have to wait a whole minute: too long for a dial
	err := l.Command("p", "z", "generate", dialMaxWait)
	var te *ThrottledError
	if !errors.As(err, &te) {
		t.Fatalf("expected ThrottledError, got %v", err)
	}
	if te.Wait != time.Minute {
		t.Fatalf("expected a one minute wait, got %s", te.Wait)
	}

	// a keep-alive may wait longer, but not this long either
	if err := l.Command("p", "z", "extend", extendMaxWait); !errors.As(err, &te) {
		t.Fatalf("expected ThrottledError for extend, got %v", err)
	}

	// fifty seconds later the wait is ten seconds, which an extend can afford
	*now = now.Add(50 * time.Second)
	if err := l.Command("p", "z", "extend", extendMaxWait); err != nil {
		t.Fatalf("extend after 50 s: %v", err)
	}
	if len(*slept) != 1 || (*slept)[0] != 10*time.Second {
		t.Fatalf("expected one 10 s sleep, got %v", *slept)
	}
}

func TestCommandPerDeviceBudget(t *testing.T) {
	l, _, _ := testLimiter(time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))

	for i := 0; i < commandPerDevicePerMin; i++ {
		if err := l.Command("p", "cam", "generate", dialMaxWait); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	// same device, same command: over the 5 QPM per-device command limit
	var te *ThrottledError
	if err := l.Command("p", "cam", "generate", dialMaxWait); !errors.As(err, &te) {
		t.Fatalf("expected ThrottledError, got %v", err)
	}
	// same device, different command: only the project budget applies and it
	// still has room
	if err := l.Command("p", "cam", "extend", dialMaxWait); err != nil {
		t.Fatalf("different command on same device: %v", err)
	}
}

func TestReservationsAreSpacedNotStacked(t *testing.T) {
	l, _, slept := testLimiter(time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))

	for i := 0; i < executeCommandPerMinute; i++ {
		if err := l.Command("p", "cam", "c"+string(rune('a'+i)), dialMaxWait); err != nil {
			t.Fatal(err)
		}
	}
	// nothing slept so far; every reservation sat at t0
	if len(*slept) != 0 {
		t.Fatalf("unexpected sleeps %v", *slept)
	}
	// with all ten slots booked at t0, the next two reservations both land
	// exactly one minute later: once the t0 slots age out there is room for
	// ten more, so they share t0+60s rather than queueing behind each other
	_, err1 := l.reserve([]*window{l.window("cmd:p", executeCommandPerMinute, time.Minute)}, "p", time.Hour)
	_, err2 := l.reserve([]*window{l.window("cmd:p", executeCommandPerMinute, time.Minute)}, "p", time.Hour)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	w := l.window("cmd:p", executeCommandPerMinute, time.Minute)
	if n := len(w.times); n != 12 {
		t.Fatalf("expected 12 reservations, got %d", n)
	}
	last := w.times[len(w.times)-1]
	if last.Sub(w.times[0]) != time.Minute {
		t.Fatalf("expected the extra reservations one minute after the first, got %s", last.Sub(w.times[0]))
	}
}

func TestPenaltyHoldsOffProject(t *testing.T) {
	l, now, slept := testLimiter(time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))

	l.Penalize("p", penaltyAfter429)

	var te *ThrottledError
	if err := l.Command("p", "cam", "generate", dialMaxWait); !errors.As(err, &te) {
		t.Fatalf("expected ThrottledError during penalty, got %v", err)
	}
	// another project is unaffected
	if err := l.Command("q", "cam", "generate", dialMaxWait); err != nil {
		t.Fatalf("other project: %v", err)
	}
	// a keep-alive waits out the remainder
	*now = now.Add(20 * time.Second)
	if err := l.Command("p", "cam", "extend", extendMaxWait); err != nil {
		t.Fatalf("extend during penalty: %v", err)
	}
	if len(*slept) != 1 || (*slept)[0] != 40*time.Second {
		t.Fatalf("expected a 40 s sleep, got %v", *slept)
	}
	// once served, the penalty is gone
	*now = now.Add(time.Second)
	if err := l.Command("p", "cam", "generate", dialMaxWait); err != nil {
		t.Fatalf("after penalty: %v", err)
	}
}

func TestListBudget(t *testing.T) {
	l, _, _ := testLimiter(time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))
	for i := 0; i < listPerMinute; i++ {
		if err := l.List("p", time.Second); err != nil {
			t.Fatal(err)
		}
	}
	var te *ThrottledError
	if err := l.List("p", time.Second); !errors.As(err, &te) {
		t.Fatalf("expected ThrottledError, got %v", err)
	}
}

func TestWindowPrunesOldReservations(t *testing.T) {
	w := &window{limit: 2, per: time.Minute}
	t0 := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	w.add(t0)
	w.add(t0.Add(10 * time.Second))
	if e := w.earliest(t0.Add(30 * time.Second)); e != t0.Add(time.Minute) {
		t.Fatalf("expected next slot at t0+60s, got %s", e.Sub(t0))
	}
	if e := w.earliest(t0.Add(61 * time.Second)); e != t0.Add(61*time.Second) {
		t.Fatalf("expected an immediate slot after the first aged out, got %s", e.Sub(t0))
	}
	if len(w.times) != 1 {
		t.Fatalf("expected the aged-out reservation pruned, have %d", len(w.times))
	}
}

// Keep-alives must leave room for dials: with nine sessions extending at
// once, only seven go now and a dial still has slots.
func TestBackgroundLeavesDialReserve(t *testing.T) {
	l, _, slept := testLimiter(time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC))

	var throttled int
	for i := 0; i < 9; i++ {
		dev := string(rune('a' + i))
		err := l.CommandBackground("p", dev, "extend", 0) // no waiting: count who gets a slot now
		var te *ThrottledError
		if errors.As(err, &te) {
			throttled++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if want := 9 - (executeCommandPerMinute - dialReserve); throttled != want {
		t.Fatalf("expected %d keep-alives held back, got %d", want, throttled)
	}
	for i := 0; i < dialReserve; i++ {
		if err := l.Command("p", "cam", "generate", dialMaxWait); err != nil {
			t.Fatalf("dial %d should still have a reserved slot: %v", i, err)
		}
	}
	var te *ThrottledError
	if err := l.Command("p", "cam2", "generate", dialMaxWait); !errors.As(err, &te) {
		t.Fatalf("budget should now be exhausted, got %v", err)
	}
	if len(*slept) != 0 {
		t.Fatalf("no sleeps expected, got %v", *slept)
	}
}
