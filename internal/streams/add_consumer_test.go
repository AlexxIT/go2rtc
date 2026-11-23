package streams

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

type testProducer struct {
	core.Connection
	onGetMedias func()
}

func (p *testProducer) GetMedias() []*core.Media {
	if p.onGetMedias != nil {
		p.onGetMedias()
		p.onGetMedias = nil
	}
	return nil
}

func (p *testProducer) Start() error {
	return nil
}

type testConsumer struct {
	core.Connection
}

func (c *testConsumer) AddTrack(*core.Media, *core.Codec, *core.Receiver) error {
	return nil
}

func TestAddConsumerProducerAddedConcurrently(t *testing.T) {
	stream := NewStream(nil)

	// A producer (e.g. an RTSP publisher) is added to the stream while
	// AddConsumer is still matching the consumer medias.
	stream.AddProducer(&testProducer{onGetMedias: func() {
		stream.AddProducer(&testProducer{})
	}})

	cons := &testConsumer{}
	cons.Medias = []*core.Media{
		{Kind: core.KindVideo, Direction: core.DirectionSendonly},
		{Kind: core.KindAudio, Direction: core.DirectionSendonly},
	}

	require.NotPanics(t, func() {
		require.Error(t, stream.AddConsumer(cons))
	})
}
