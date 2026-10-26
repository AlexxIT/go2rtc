package streams

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/stretchr/testify/require"
)

// fakeConn is a core.Producer whose Start() behaviour is scripted.
type fakeConn struct {
	core.Connection
	start func() error
	stops atomic.Int32
}

func (f *fakeConn) Start() error { return f.start() }

func (f *fakeConn) Stop() error {
	f.stops.Add(1)
	return f.Connection.Stop()
}

func (p *Producer) snapshot() (s state, conn core.Producer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state, p.conn
}

// TestForceResetAfterConsecutiveConnStateErrors: connFailLimit consecutive
// rtsp.ErrStartFromConn failures must tear the producer down to stateNone
// (conn closed and nil) instead of hot-looping dial, error, dial forever.
// That is the signature of a dead camera: repeated `start from CONN state`.
func TestForceResetAfterConsecutiveConnStateErrors(t *testing.T) {
	var dials atomic.Int32

	HandleFunc("wedgeconn", func(url string) (core.Producer, error) {
		dials.Add(1)
		return &fakeConn{start: func() error { return rtsp.ErrStartFromConn }}, nil
	})

	p := NewProducer("wedgeconn:cam")
	require.NoError(t, p.Dial())

	p.mu.Lock()
	p.state = stateTracks // pretend a consumer got its track
	p.mu.Unlock()

	p.start()

	require.Eventually(t, func() bool {
		s, conn := p.snapshot()
		return s == stateNone && conn == nil
	}, 5*time.Second, 10*time.Millisecond, "producer must force-reset to stateNone")

	// initial dial + (connFailLimit-1) redials, then reset instead of redial
	require.Equal(t, int32(connFailLimit), dials.Load())

	// and the producer must be dialable again (fresh consumer recovers)
	require.NoError(t, p.Dial())
	s, conn := p.snapshot()
	require.Equal(t, stateMedias, s)
	require.NotNil(t, conn)
}

// TestNoResetOnOtherErrors: non-CONN errors must NOT force-reset. They take
// the ordinary supervised reconnect path and the counter resets every time.
func TestNoResetOnOtherErrors(t *testing.T) {
	var dials atomic.Int32

	HandleFunc("flaky", func(url string) (core.Producer, error) {
		dials.Add(1)
		return &fakeConn{start: func() error { return errors.New("eof") }}, nil
	})

	p := NewProducer("flaky:cam")
	require.NoError(t, p.Dial())

	p.mu.Lock()
	p.state = stateTracks
	p.mu.Unlock()

	p.start()

	require.Eventually(t, func() bool {
		return dials.Load() >= connFailLimit+2
	}, 5*time.Second, 10*time.Millisecond, "reconnect loop must keep running")

	s, _ := p.snapshot()
	require.NotEqual(t, stateNone, s, "must not force-reset on non-CONN errors")

	p.stop() // shut the loop down
}

// TestConnFailCounterResetBySuccess: a successful Start() between CONN-state
// errors resets the counter, so a recovering producer is never torn down.
func TestConnFailCounterResetBySuccess(t *testing.T) {
	var dials atomic.Int32

	HandleFunc("blip", func(url string) (core.Producer, error) {
		n := dials.Add(1)
		return &fakeConn{start: func() error {
			if n%2 == 1 {
				return rtsp.ErrStartFromConn
			}
			// "healthy" session: runs for a bit, then clean exit
			time.Sleep(50 * time.Millisecond)
			return nil
		}}, nil
	})

	p := NewProducer("blip:cam")
	require.NoError(t, p.Dial())

	p.mu.Lock()
	p.state = stateTracks
	p.mu.Unlock()

	p.start()

	require.Eventually(t, func() bool {
		return dials.Load() >= 2*connFailLimit
	}, 5*time.Second, 10*time.Millisecond)

	s, _ := p.snapshot()
	require.NotEqual(t, stateNone, s, "alternating errors must never accumulate to a reset")

	p.stop()
}

// TestReconnectDoesNotHoldMutexWhileDialing: regression test for the
// deadlock signature: GetProducer blocking (a dead-camera exec pipe
// probe) must NOT freeze the producer mutex; stop() has to complete promptly
// and the late dial result must be discarded (conn stopped).
func TestReconnectDoesNotHoldMutexWhileDialing(t *testing.T) {
	block := make(chan struct{})
	late := &fakeConn{start: func() error { return nil }}
	first := true

	HandleFunc("hangdial", func(url string) (core.Producer, error) {
		if first {
			first = false
			return &fakeConn{start: func() error { return errors.New("eof") }}, nil
		}
		<-block // simulate exec pipe probe blocking on a dead camera
		return late, nil
	})

	p := NewProducer("hangdial:cam")
	require.NoError(t, p.Dial())

	p.mu.Lock()
	p.state = stateTracks
	p.mu.Unlock()

	p.start() // worker errors instantly, reconnect runs, 2nd dial blocks

	time.Sleep(100 * time.Millisecond) // let reconnect reach the blocked dial

	// with the old code this deadlocked forever on p.mu
	done := make(chan struct{})
	go func() {
		p.stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() blocked behind a dialing reconnect: producer mutex deadlock")
	}

	// unblock the stale dial: its conn must be stopped, not installed
	close(block)
	require.Eventually(t, func() bool {
		return late.stops.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "stale dial result must be stopped and discarded")

	_, conn := p.snapshot()
	require.Nil(t, conn)
}

// TestDialDoesNotHoldMutexWhileDialing: regression test for the last
// GetProducer-under-p.mu call site. A blocked first dial must not freeze the
// producer: stop() and a second consumer's Dial have to make progress.
func TestDialDoesNotHoldMutexWhileDialing(t *testing.T) {
	block := make(chan struct{})
	late := &fakeConn{start: func() error { return nil }}

	HandleFunc("hangfirstdial", func(url string) (core.Producer, error) {
		<-block
		return late, nil
	})

	p := NewProducer("hangfirstdial:cam")

	dialed := make(chan error, 1)
	go func() { dialed <- p.Dial() }()

	time.Sleep(100 * time.Millisecond) // let the dial block

	// with upstream code this parks on p.mu until the dial returns
	stopped := make(chan struct{})
	go func() {
		p.stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() blocked behind Dial's GetProducer: producer mutex freeze")
	}

	close(block)
	require.Error(t, <-dialed, "a dial that lands after stop() must not install its conn")

	require.Eventually(t, func() bool {
		return late.stops.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "superseded dial result must be stopped")

	_, conn := p.snapshot()
	require.Nil(t, conn)
}

// TestConcurrentDialIsSingleFlight: N consumers racing to attach to the same
// cold producer must spawn exactly ONE underlying connection (one exec child),
// not one each.
func TestConcurrentDialIsSingleFlight(t *testing.T) {
	var dials atomic.Int32
	release := make(chan struct{})

	HandleFunc("slowdial", func(url string) (core.Producer, error) {
		dials.Add(1)
		<-release
		return &fakeConn{start: func() error { return nil }}, nil
	})

	p := NewProducer("slowdial:cam")

	const n = 8
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() { errs <- p.Dial() }()
	}

	time.Sleep(200 * time.Millisecond)
	close(release)

	for i := 0; i < n; i++ {
		select {
		case err := <-errs:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("concurrent Dial did not return")
		}
	}

	require.Equal(t, int32(1), dials.Load(), "single-flight: exactly one GetProducer")

	s, conn := p.snapshot()
	require.Equal(t, stateMedias, s)
	require.NotNil(t, conn)
}

// TestDialTimeoutAbandonsAndRecovers: a handler that never returns must not
// strand the producer. The dial is abandoned with ErrDialTimeout, waiters are
// woken with the same error, the producer stays dialable, and the orphaned
// connection is reaped if it ever materialises.
func TestDialTimeoutAbandonsAndRecovers(t *testing.T) {
	old := dialTimeout
	dialTimeout = 200 * time.Millisecond
	defer func() { dialTimeout = old }()

	block := make(chan struct{})
	orphan := &fakeConn{start: func() error { return nil }}
	var dials atomic.Int32

	HandleFunc("neverdial", func(url string) (core.Producer, error) {
		if dials.Add(1) == 1 {
			<-block // first dial hangs forever
			return orphan, nil
		}
		return &fakeConn{start: func() error { return nil }}, nil
	})

	p := NewProducer("neverdial:cam")

	first := make(chan error, 1)
	go func() { first <- p.Dial() }()

	time.Sleep(50 * time.Millisecond)
	waiter := make(chan error, 1)
	go func() { waiter <- p.Dial() }() // parks on the in-flight dial

	select {
	case err := <-first:
		require.ErrorIs(t, err, ErrDialTimeout)
	case <-time.After(2 * time.Second):
		t.Fatal("Dial did not honour dialTimeout")
	}

	select {
	case err := <-waiter:
		require.ErrorIs(t, err, ErrDialTimeout, "waiter must be woken with an error, not parked")
	case <-time.After(2 * time.Second):
		t.Fatal("waiter parked on an abandoned dial")
	}

	// producer must be dialable again rather than permanently wedged
	require.NoError(t, p.Dial())
	s, conn := p.snapshot()
	require.Equal(t, stateMedias, s)
	require.NotNil(t, conn)

	// the abandoned dial's connection is reaped when it finally arrives
	close(block)
	require.Eventually(t, func() bool {
		return orphan.stops.Load() == 1
	}, 2*time.Second, 10*time.Millisecond, "orphaned dial result must be stopped")
}
