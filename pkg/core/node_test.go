package core

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func requireKeyframe(t *testing.T, calls <-chan struct{}) {
	t.Helper()

	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("keyframe handler was not called")
	}
}

func TestRequestKeyframe(t *testing.T) {
	calls := make(chan struct{}, 1)
	receiver := NewReceiver(nil, &Codec{})
	receiver.OnKeyframe(func() { calls <- struct{}{} })

	sender := NewSender(nil, &Codec{})
	sender.Bind(receiver)

	sender.RequestKeyframe()
	requireKeyframe(t, calls)
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
	oldCalls := make(chan struct{}, 1)
	newCalls := make(chan struct{}, 1)
	old := NewReceiver(nil, &Codec{})
	old.OnKeyframe(func() { oldCalls <- struct{}{} })
	track := NewReceiver(nil, &Codec{})
	track.OnKeyframe(func() { newCalls <- struct{}{} })

	sender := NewSender(nil, &Codec{})
	sender.Bind(old)
	old.Replace(track)

	sender.RequestKeyframe()
	requireKeyframe(t, newCalls)
	require.Empty(t, oldCalls)
}

// requests run in the RTCP goroutine, while other goroutines bind and close
func TestRequestKeyframeConcurrent(t *testing.T) {
	calls := make(chan struct{}, 1000)
	handler := func() { calls <- struct{}{} }
	a := NewReceiver(nil, &Codec{})
	a.OnKeyframe(handler)
	b := NewReceiver(nil, &Codec{})
	b.OnKeyframe(handler)

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
			a.OnKeyframe(handler)
		}
		sender.Close()
	}()
	wg.Wait()

	for range 1000 {
		requireKeyframe(t, calls)
	}
	require.Empty(t, calls)
}
