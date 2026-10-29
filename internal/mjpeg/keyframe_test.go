package mjpeg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/rs/zerolog"
)

// silentProducer connects fine, but never sends a single packet - exactly what
// the camera on 192.168.1.180 looked like from the device.
type silentProducer struct {
	core.Connection
	stop chan struct{}
}

func newSilentProducer() *silentProducer {
	return &silentProducer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "silent",
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{{Name: core.CodecH264, ClockRate: 90000, PayloadType: 96}},
				},
			},
		},
		stop: make(chan struct{}),
	}
}

func (p *silentProducer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	track := core.NewReceiver(media, codec)
	p.Receivers = append(p.Receivers, track)
	return track, nil
}

func (p *silentProducer) Start() error {
	<-p.stop // block like a real producer waiting on the socket
	return nil
}

func (p *silentProducer) Stop() error {
	select {
	case <-p.stop:
	default:
		close(p.stop)
	}
	return p.Connection.Stop()
}

func consumerCount(t *testing.T, s *streams.Stream) int {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal stream: %v", err)
	}
	var v struct {
		Consumers []json.RawMessage `json:"consumers"`
	}
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal stream: %v", err)
	}
	return len(v.Consumers)
}

// TestKeyframeTimeout is the regression test for the bug that wedged the device:
// a snapshot from a camera that never sends a keyframe used to block forever,
// leaking the consumer and pinning the producer in an endless reconnect loop.
func TestKeyframeTimeout(t *testing.T) {
	log = zerolog.Nop()

	streams.HandleFunc("silent", func(source string) (core.Producer, error) {
		return newSilentProducer(), nil
	})

	const name = "silent-cam"
	stream, err := streams.New(name, "silent:never-sends-a-frame")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	defer streams.Delete(name)

	req := httptest.NewRequest(http.MethodGet, "/api/frame.jpeg?src="+name+"&timeout=1", nil)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handlerKeyframe(rec, req)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handlerKeyframe did not return - it is still blocking on WriteTo")
	}

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusGatewayTimeout)
	}

	if n := consumerCount(t, stream); n != 0 {
		t.Errorf("stream kept %d consumer(s) after timeout, want 0 - consumer leaked", n)
	}
}

// TestKeyframeClientDisconnect covers the other half: the client goes away while
// go2rtc is still waiting for a keyframe.
func TestKeyframeClientDisconnect(t *testing.T) {
	log = zerolog.Nop()

	streams.HandleFunc("silent", func(source string) (core.Producer, error) {
		return newSilentProducer(), nil
	})

	const name = "silent-cam-2"
	stream, err := streams.New(name, "silent:never-sends-a-frame")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	defer streams.Delete(name)

	req := httptest.NewRequest(http.MethodGet, "/api/frame.jpeg?src="+name, nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handlerKeyframe(rec, req)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel() // client drops the connection

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handlerKeyframe ignored client disconnect")
	}

	if n := consumerCount(t, stream); n != 0 {
		t.Errorf("stream kept %d consumer(s) after disconnect, want 0", n)
	}
}
