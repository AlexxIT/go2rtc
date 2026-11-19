package mjpeg

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

// silentProducer offers an H264 video track and then never sends anything, the
// behaviour of a source that cannot produce a keyframe. HomeKit cameras hit this
// when the accessory stops after a partial access unit.
type silentProducer struct {
	medias   []*core.Media
	receiver *core.Receiver
}

func newSilentProducer() *silentProducer {
	return &silentProducer{
		medias: []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96},
				},
			},
		},
	}
}

func (p *silentProducer) GetMedias() []*core.Media { return p.medias }

func (p *silentProducer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if p.receiver == nil {
		p.receiver = core.NewReceiver(media, codec)
	}
	return p.receiver, nil
}

func (p *silentProducer) Start() error { return nil }

func (p *silentProducer) Stop() error {
	if p.receiver != nil {
		p.receiver.Close()
	}
	return nil
}

func consumerCount(t *testing.T, stream *streams.Stream) int {
	t.Helper()

	b, err := json.Marshal(stream)
	require.NoError(t, err)

	var info struct {
		Consumers []json.RawMessage `json:"consumers"`
	}
	require.NoError(t, json.Unmarshal(b, &info))
	return len(info.Consumers)
}

// handlerKeyframe blocks in cons.WriteTo waiting for a keyframe. If the client
// goes away first - or the source never produces one - the handler must still
// release the consumer, otherwise the stream keeps a consumer forever and
// stopProducers never closes the upstream connection. On a battery powered
// camera that pins the device awake indefinitely.
func TestKeyframeHandlerReleasesConsumerOnClientDisconnect(t *testing.T) {
	log = zerolog.Nop()

	streams.HandleFunc("silent", func(string) (core.Producer, error) {
		return newSilentProducer(), nil
	})

	stream, err := streams.New("test_silent_cam", "silent:cam")
	require.NoError(t, err)
	require.NotNil(t, stream)
	t.Cleanup(func() { streams.Delete("test_silent_cam") })

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/frame.jpeg?src=test_silent_cam", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handlerKeyframe(rec, req)
	}()

	// the consumer attaches while waiting for a keyframe that never arrives
	require.Eventually(t, func() bool {
		return consumerCount(t, stream) == 1
	}, 5*time.Second, 10*time.Millisecond, "consumer never attached")

	// the client gives up, as Home Assistant does at its 10s image timeout
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after the client disconnected")
	}

	require.Equal(t, 0, consumerCount(t, stream), "consumer leaked after client disconnect")
}
