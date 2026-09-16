package core

import (
	"testing"
	"time"
)

// TestWriteBufferCloseBeforeWriteTo guards the race that the snapshot timeout
// would otherwise hit: if Close() lands before WriteTo(), add() used to start
// waiting on an already-closed buffer and WriteTo would block forever.
func TestWriteBufferCloseBeforeWriteTo(t *testing.T) {
	wr := NewWriteBuffer(nil)

	if err := wr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = wr.WriteTo(&OnceBuffer{})
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("WriteTo blocked after Close - deadlock")
	}
}

// TestWriteBufferCloseUnblocksWriteTo is the ordinary path: WriteTo is already
// waiting when Close arrives.
func TestWriteBufferCloseUnblocksWriteTo(t *testing.T) {
	wr := NewWriteBuffer(nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = wr.WriteTo(&OnceBuffer{})
	}()

	time.Sleep(100 * time.Millisecond)
	if err := wr.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not unblock WriteTo")
	}
}
