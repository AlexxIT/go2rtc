package streams

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// fakeKeyframeProducer is a recvonly video+audio producer that counts keyframe requests.
type fakeKeyframeProducer struct {
	calls atomic.Int32
	done  chan struct{}
	once  sync.Once
}

func newFakeKeyframeProducer() *fakeKeyframeProducer {
	return &fakeKeyframeProducer{done: make(chan struct{})}
}

func (f *fakeKeyframeProducer) GetMedias() []*core.Media {
	return []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{{Name: core.CodecH264, ClockRate: 90000}}},
		{Kind: core.KindAudio, Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}}},
	}
}

func (f *fakeKeyframeProducer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return core.NewReceiver(media, codec), nil
}

func (f *fakeKeyframeProducer) Start() error { <-f.done; return nil }
func (f *fakeKeyframeProducer) Stop() error  { f.once.Do(func() { close(f.done) }); return nil }

func (f *fakeKeyframeProducer) RequestKeyframe() error {
	f.calls.Add(1)
	return nil
}

// plainProducer has no RequestKeyframe method.
type plainProducer struct{ fakeKeyframeProducer }

func (*plainProducer) RequestKeyframe() {}

// fakeKeyframeConsumer is a sendonly video+audio consumer that stores handlers per media ID.
type fakeKeyframeConsumer struct {
	mu       sync.Mutex
	handlers map[string]func()
}

func (c *fakeKeyframeConsumer) GetMedias() []*core.Media {
	return []*core.Media{
		{ID: "v", Kind: core.KindVideo, Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{{Name: core.CodecH264, ClockRate: 90000}}},
		{ID: "a", Kind: core.KindAudio, Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{{Name: core.CodecPCMA, ClockRate: 8000}}},
	}
}

// AddTrack binds a sender so the producer counts as having readers
// (otherwise AddConsumer stops it again).
func (c *fakeKeyframeConsumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	// Bind is deprecated, but it is how a sender counts as a reader of the track here
	core.NewSender(media, codec).Bind(track) //nolint:staticcheck // SA1019: see above
	return nil
}

func (c *fakeKeyframeConsumer) Stop() error { return nil }

func (c *fakeKeyframeConsumer) OnKeyframeRequest(media *core.Media, handler func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handlers == nil {
		c.handlers = map[string]func(){}
	}
	c.handlers[media.ID] = handler
}

func (c *fakeKeyframeConsumer) handler(id string) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.handlers[id]
}

// newKeyframeStream registers the kftest scheme (process-wide state, cleaned up
// afterwards) and returns a stream whose single producer wraps prod.
func newKeyframeStream(t *testing.T, prod core.Producer) *Stream {
	t.Helper()
	HandleFunc("kftest", func(string) (core.Producer, error) { return prod, nil })
	stream := NewStream("kftest:a")
	t.Cleanup(func() {
		delete(handlers, "kftest")
		for _, p := range stream.producers {
			// invalidate the worker so reconnect does not run after the test
			p.mu.Lock()
			p.workerID++
			p.mu.Unlock()
		}
		_ = prod.Stop() //nolint:staticcheck // SA1019: Stop is the core.Producer method
	})
	return stream
}

func TestAddConsumerWiresKeyframeForVideo(t *testing.T) {
	prod := newFakeKeyframeProducer()
	stream := newKeyframeStream(t, prod)

	cons := &fakeKeyframeConsumer{}
	require.NoError(t, stream.AddConsumer(cons))

	require.Nil(t, cons.handler("a"), "audio must not get a keyframe handler")
	h := cons.handler("v")
	require.NotNil(t, h)

	h()
	require.EqualValues(t, 1, prod.calls.Load())
}

func TestAddConsumerKeyframeSharedByConsumers(t *testing.T) {
	prod := newFakeKeyframeProducer()
	stream := newKeyframeStream(t, prod)

	c1, c2 := &fakeKeyframeConsumer{}, &fakeKeyframeConsumer{}
	require.NoError(t, stream.AddConsumer(c1))
	require.NoError(t, stream.AddConsumer(c2))

	c1.handler("v")()
	c2.handler("v")()
	require.EqualValues(t, 2, prod.calls.Load())
}

func TestProducerRequestKeyframeFollowsConn(t *testing.T) {
	first := newFakeKeyframeProducer()
	stream := newKeyframeStream(t, first)

	cons := &fakeKeyframeConsumer{}
	require.NoError(t, stream.AddConsumer(cons))

	// simulate reconnect: the producer gets a new connection
	second := newFakeKeyframeProducer()
	p := stream.producers[0]
	p.mu.Lock()
	p.conn = second
	p.mu.Unlock()

	cons.handler("v")()
	require.EqualValues(t, 0, first.calls.Load())
	require.EqualValues(t, 1, second.calls.Load())
}

func TestProducerRequestKeyframeDoesNotBlock(t *testing.T) {
	p := &Producer{conn: newFakeKeyframeProducer()}

	p.mu.Lock()
	defer p.mu.Unlock()

	errc := make(chan error, 1)
	go func() { errc <- p.RequestKeyframe() }()

	select {
	case err := <-errc:
		require.ErrorIs(t, err, errProducerBusy)
	case <-time.After(50 * time.Millisecond):
		t.Fatal("RequestKeyframe blocked on the producer mutex")
	}
}

func TestProducerRequestKeyframeUnsupported(t *testing.T) {
	p := &Producer{conn: &plainProducer{}}
	err := p.RequestKeyframe()
	require.True(t, errors.Is(err, core.ErrKeyframeUnsupported))

	p = &Producer{}
	err = p.RequestKeyframe()
	require.True(t, errors.Is(err, core.ErrKeyframeUnsupported))
}
