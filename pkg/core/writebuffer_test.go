package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A WriteTo still blocked on a keyframe must unblock on Close, or the consumer leaks.
func TestWriteBufferCloseUnblocksWriteTo(t *testing.T) {
	wb := NewWriteBuffer(nil)

	returned := make(chan struct{})
	go func() {
		_, _ = wb.WriteTo(&OnceBuffer{}) // no keyframe is ever written
		close(returned)
	}()

	// WriteTo must still be blocked: no keyframe, no Close yet.
	select {
	case <-returned:
		t.Fatal("WriteTo returned before any keyframe or Close")
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, wb.Close())

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("WriteTo did not unblock after Close - the consumer would leak")
	}
}

// Close before WriteTo registers its wait must not block WriteTo forever.
func TestWriteBufferCloseBeforeWriteTo(t *testing.T) {
	wb := NewWriteBuffer(nil)
	require.NoError(t, wb.Close())

	returned := make(chan struct{})
	go func() {
		_, _ = wb.WriteTo(&OnceBuffer{})
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("WriteTo blocked after Close-before-WriteTo - the race window leaks the consumer")
	}
}
