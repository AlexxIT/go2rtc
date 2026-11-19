package core

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestKeyframe(t *testing.T) {
	var calls int
	receiver := NewReceiver(nil, &Codec{})
	receiver.OnKeyframe(func() { calls++ })

	sender := NewSender(nil, &Codec{})
	sender.Bind(receiver)

	sender.RequestKeyframe()
	require.Equal(t, 1, calls)
}

func TestRequestKeyframeNoHandler(t *testing.T) {
	receiver := NewReceiver(nil, &Codec{})
	sender := NewSender(nil, &Codec{})
	sender.Bind(receiver)

	sender.RequestKeyframe() // nothing happens
	(&Node{}).RequestKeyframe()
}

// after the source was replaced (ex. reconnect), requests go to the new source
func TestRequestKeyframeMoveNode(t *testing.T) {
	var oldCalls, newCalls int
	old := NewReceiver(nil, &Codec{})
	old.OnKeyframe(func() { oldCalls++ })
	track := NewReceiver(nil, &Codec{})
	track.OnKeyframe(func() { newCalls++ })

	sender := NewSender(nil, &Codec{})
	sender.Bind(old)
	old.Replace(track)

	sender.RequestKeyframe()
	require.Equal(t, 0, oldCalls)
	require.Equal(t, 1, newCalls)
}

// requests run in the RTCP goroutine, while other goroutines bind and close
func TestRequestKeyframeConcurrent(t *testing.T) {
	var calls atomic.Int32
	a := NewReceiver(nil, &Codec{})
	a.OnKeyframe(func() { calls.Add(1) })
	b := NewReceiver(nil, &Codec{})
	b.OnKeyframe(func() { calls.Add(1) })

	sender := NewSender(nil, &Codec{})
	sender.Bind(a)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1000 {
			sender.RequestKeyframe()
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 1000 {
			if i%2 == 0 {
				MoveNode(&b.Node, &a.Node)
			} else {
				MoveNode(&a.Node, &b.Node)
			}
			a.OnKeyframe(func() { calls.Add(1) })
		}
		sender.Close()
	}()
	wg.Wait()

	require.EqualValues(t, 1000, calls.Load())
}
