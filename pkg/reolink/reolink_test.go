package reolink

import (
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func TestRTPTimestampGuardMonotonic(t *testing.T) {
	guard := &rtpTimestampGuard{}

	// Initial timestamp
	ts1 := guard.next(1000)
	if ts1 != 1000 {
		t.Fatalf("expected 1000, got %d", ts1)
	}

	// Normal forward step
	ts2 := guard.next(4000)
	if ts2 != 4000 {
		t.Fatalf("expected 4000, got %d", ts2)
	}

	// Duplicate timestamp (delta == 0)
	ts3 := guard.next(4000)
	if ts3 != 4001 {
		t.Fatalf("expected 4001 for duplicate, got %d", ts3)
	}

	// Subsequent normal packet advances relative to its own delta
	ts4 := guard.next(7000)
	if ts4 != 7001 {
		t.Fatalf("expected 7001, got %d", ts4)
	}

	// Backward timestamp jump (e.g. camera clock reset to 0)
	ts5 := guard.next(0)
	if ts5 != 7002 {
		t.Fatalf("expected 7002 for backward jump, got %d", ts5)
	}

	// Resumed stream from new timeline
	ts6 := guard.next(3000)
	if ts6 != 10002 {
		t.Fatalf("expected 10002, got %d", ts6)
	}
}

func TestRTPTimestampGuardHugeForwardJump(t *testing.T) {
	guard := &rtpTimestampGuard{}

	guard.next(1000)
	// Jump forward by > 10 seconds (900000 ticks at 90kHz)
	ts := guard.next(2_000_000)
	if ts != 1001 {
		t.Fatalf("expected forward runaway jump to be guarded to 1001, got %d", ts)
	}

	// Subsequent packet advances from new timeline
	ts2 := guard.next(2_003_000)
	if ts2 != 4001 {
		t.Fatalf("expected 4001, got %d", ts2)
	}
}

func TestSetupBackchannelDisabled(t *testing.T) {
	c := &Client{
		backchannelEnabled: false,
	}
	err := c.SetupBackchannel()
	if err == nil || err.Error() != "reolink: talkback is disabled" {
		t.Fatalf("expected 'reolink: talkback is disabled', got %v", err)
	}
}

func TestSetupBackchannelClosed(t *testing.T) {
	c := &Client{
		backchannelEnabled: true,
		closed:             true,
	}
	err := c.SetupBackchannel()
	if err == nil || err.Error() != "reolink: client is closed" {
		t.Fatalf("expected 'reolink: client is closed', got %v", err)
	}
}

func TestTalkbackNonBlockingDrop(t *testing.T) {
	c := &Client{
		backchannelEnabled: true,
	}
	// Emulate talkMu being locked during background setup
	c.talkMu.Lock()

	media := &core.Media{Kind: core.KindAudio}
	codec := &core.Codec{Name: core.CodecPCMU}
	track := core.NewReceiver(media, codec)

	if err := c.AddTrack(media, codec, track); err != nil {
		t.Fatalf("AddTrack failed: %v", err)
	}

	// Calling Handler while talkMu is locked should return immediately (dropping the packet)
	// and NOT block on the lock.
	done := make(chan struct{})
	go func() {
		c.sender.Handler(&core.Packet{Payload: []byte{0, 1, 2, 3}})
		close(done)
	}()

	select {
	case <-done:
		// success: handler returned immediately without blocking
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("c.sender.Handler blocked while talkMu was locked!")
	}

	c.talkMu.Unlock()
}

