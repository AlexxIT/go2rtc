package streams

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// fakeProducer is a minimal core.Producer whose Start() behavior is
// controlled by the test, so we can simulate a producer that fails
// almost immediately, the way a flaky camera answers DESCRIBE fine but
// resets the connection on PLAY.
type fakeProducer struct {
	startErr func() error
}

func (f *fakeProducer) GetMedias() []*core.Media { return nil }

func (f *fakeProducer) GetTrack(*core.Media, *core.Codec) (*core.Receiver, error) {
	return nil, nil
}

func (f *fakeProducer) Start() error { return f.startErr() }

func (f *fakeProducer) Stop() error { return nil }

// TestReconnectBacksOffOnImmediateFailure reproduces
// https://github.com/AlexxIT/go2rtc/issues/2468 - a producer whose
// Start() fails immediately every time (GetProducer/DESCRIBE succeeds,
// but PLAY resets) must still back off between reconnect attempts
// instead of looping at zero delay, which was driving CPU/RAM up.
func TestReconnectBacksOffOnImmediateFailure(t *testing.T) {
	const scheme = "faketest2468immediate"

	var mu sync.Mutex
	var attempts []time.Time

	HandleFunc(scheme, func(string) (core.Producer, error) {
		mu.Lock()
		attempts = append(attempts, time.Now())
		mu.Unlock()

		return &fakeProducer{
			startErr: func() error {
				return errors.New("connection reset by peer")
			},
		}, nil
	})

	p := NewProducer(scheme + "://fake")
	if err := p.Dial(); err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	p.state = stateTracks

	p.start()

	// Enough time for ~2 reconnect attempts if the 1s backoff (retry<5)
	// is respected, but nowhere near enough for 2 if it's a zero-delay
	// loop - a buggy run produces dozens of attempts in this window.
	time.Sleep(2500 * time.Millisecond)

	mu.Lock()
	n := len(attempts)
	var gap time.Duration
	if n >= 2 {
		gap = attempts[1].Sub(attempts[0])
	}
	mu.Unlock()

	if n < 2 {
		t.Fatalf("expected at least 2 reconnect attempts in 2.5s, got %d", n)
	}
	if gap < 500*time.Millisecond {
		t.Fatalf("reconnect attempts were only %v apart, expected ~1s backoff - looks like a zero-delay retry loop", gap)
	}
	if n > 4 {
		t.Fatalf("got %d reconnect attempts in 2.5s, way more than the ~1s backoff allows - looks like a zero-delay retry loop", n)
	}
}
