package streams

import (
	"context"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// stopConsumer records Stop calls so tests can check the live consumer was closed.
type stopConsumer struct {
	stopped int
}

func (c *stopConsumer) GetMedias() []*core.Media                                { return nil }
func (c *stopConsumer) AddTrack(*core.Media, *core.Codec, *core.Receiver) error { return nil }

func (c *stopConsumer) Stop() error {
	c.stopped++
	return nil
}

// addPublish installs a fake active publish and returns its consumer and context.
func addPublish(s *Stream, url string) (*publishHandle, *stopConsumer, context.Context) {
	cons := &stopConsumer{}
	ctx, cancel := context.WithCancel(context.Background())
	h := &publishHandle{cancel: cancel, cons: cons}

	if s.publishes == nil {
		s.publishes = map[string]*publishHandle{}
	}
	s.publishes[url] = h

	return h, cons, ctx
}

func TestStopPublish(t *testing.T) {
	s := &Stream{}
	_, cons, ctx := addPublish(s, "rtmp://host/a")

	s.StopPublish("rtmp://host/a")

	require.Error(t, ctx.Err(), "retry loop should be cancelled")
	require.Equal(t, 1, cons.stopped, "live consumer should be closed")
	require.Empty(t, s.publishes, "handle should be removed")
}

func TestStopPublishUnknownURL(t *testing.T) {
	s := &Stream{}
	_, cons, ctx := addPublish(s, "rtmp://host/a")

	s.StopPublish("rtmp://host/other")

	require.NoError(t, ctx.Err())
	require.Equal(t, 0, cons.stopped)
	require.Len(t, s.publishes, 1)
}

func TestStopAllPublish(t *testing.T) {
	s := &Stream{}
	_, consA, ctxA := addPublish(s, "rtmp://host/a")
	_, consB, ctxB := addPublish(s, "rtmp://host/b")

	s.StopAllPublish()

	require.Error(t, ctxA.Err())
	require.Error(t, ctxB.Err())
	require.Equal(t, 1, consA.stopped)
	require.Equal(t, 1, consB.stopped)
	require.Empty(t, s.publishes)
}

func TestRemoveHandleKeepsNewer(t *testing.T) {
	s := &Stream{}
	old, _, _ := addPublish(s, "rtmp://host/a")
	current, _, _ := addPublish(s, "rtmp://host/a") // replaces old in the map

	s.removeHandle("rtmp://host/a", old)
	require.Same(t, current, s.publishes["rtmp://host/a"], "newer handle must survive")

	s.removeHandle("rtmp://host/a", current)
	require.Empty(t, s.publishes)
}
