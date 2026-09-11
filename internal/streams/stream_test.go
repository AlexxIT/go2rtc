package streams

import (
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

// mockConsumer is a minimal core.Consumer for testing kickConsumers.
// It records how many times Stop() was called.
type mockConsumer struct {
	stops atomic.Int32
}

func (m *mockConsumer) GetMedias() []*core.Media                                { return nil }
func (m *mockConsumer) AddTrack(*core.Media, *core.Codec, *core.Receiver) error { return nil }
func (m *mockConsumer) Stop() error {
	m.stops.Add(1)
	return nil
}

// mockInternalConsumer is a mockConsumer that reports a source URL.
// Models the inner ffmpeg subprocess of a recursive pipeline like
// `ffmpeg:driveway` (subprocess reads rtsp://localhost/driveway and
// is registered as a consumer of the same stream).
type mockInternalConsumer struct {
	mockConsumer
	source string
}

// Satisfy core.Info — we only need GetSource for the kick-skip check.
func (m *mockInternalConsumer) SetProtocol(string)        {}
func (m *mockInternalConsumer) SetRemoteAddr(string)      {}
func (m *mockInternalConsumer) SetSource(string)          {}
func (m *mockInternalConsumer) SetURL(string)             {}
func (m *mockInternalConsumer) WithRequest(*http.Request) {}
func (m *mockInternalConsumer) GetSource() string         { return m.source }

func TestRecursion(t *testing.T) {
	// create stream with some source
	stream1, err := New("from_yaml", "does_not_matter")
	require.NoError(t, err)
	require.Len(t, streams, 1)

	// ask another unnamed stream that links go2rtc
	query, err := url.ParseQuery("src=rtsp://localhost:8554/from_yaml?video")
	require.NoError(t, err)
	stream2, err := GetOrPatch(query)
	require.NoError(t, err)

	// check stream is same
	require.Equal(t, stream1, stream2)
	// check stream urls is same
	require.Equal(t, stream1.producers[0].url, stream2.producers[0].url)
	require.Len(t, streams, 2)
}

func TestTempate(t *testing.T) {
	HandleFunc("rtsp", func(url string) (core.Producer, error) { return nil, nil }) // bypass HasProducer

	// config from yaml
	stream1, err := New("camera.from_hass", "ffmpeg:{input}#video=copy")
	require.NoError(t, err)
	// request from hass
	stream2, err := Patch("camera.from_hass", "rtsp://example.com")
	require.NoError(t, err)

	require.Equal(t, stream1, stream2)
	require.Equal(t, "ffmpeg:rtsp://example.com#video=copy", stream1.producers[0].url)
}

// TestKickConsumers verifies that Stream.kickConsumers calls Stop()
// exactly once on every attached consumer. This is the disconnect path
// used after a Producer reconnects so downstream RTSP clients re-DESCRIBE
// and pick up fresh codec parameters (SPS/PPS).
func TestKickConsumers(t *testing.T) {
	t.Run("no consumers - no-op, doesn't panic", func(t *testing.T) {
		s := &Stream{}
		s.kickConsumers("test")
	})

	t.Run("multiple consumers - all get Stop()", func(t *testing.T) {
		s := &Stream{}
		consumers := []*mockConsumer{{}, {}, {}}
		for _, c := range consumers {
			s.consumers = append(s.consumers, c)
		}

		s.kickConsumers("test reason")

		for i, c := range consumers {
			require.Equal(t, int32(1), c.stops.Load(),
				"consumer %d should have Stop() called exactly once", i)
		}
	})

	t.Run("kick doesn't remove consumers from list", func(t *testing.T) {
		// kickConsumers only triggers Stop(); the actual list cleanup
		// happens in RemoveConsumer when each consumer's transport
		// handler exits naturally.
		s := &Stream{}
		s.consumers = append(s.consumers, &mockConsumer{}, &mockConsumer{})

		s.kickConsumers("test")

		require.Len(t, s.consumers, 2,
			"kickConsumers should not modify s.consumers directly")
	})

	t.Run("internal-loop consumer is skipped (no infinite reconnect)", func(t *testing.T) {
		// Regression test for the recursive-pipeline reconnect loop:
		//
		//   ffmpeg:driveway producer
		//     -> ffmpeg subprocess (registers as consumer of driveway)
		//     -> kick called on producer reconnect
		//     -> kicks the inner ffmpeg subprocess (its own source)
		//     -> subprocess exits -> producer EOFs -> reconnect cycle
		//
		// Fix: consumers whose GetSource() matches one of this stream's
		// producer URLs are exempt from the kick.
		s := &Stream{
			producers: []*Producer{
				{url: "nest:?client_id=…"},
				{url: "ffmpeg:driveway"},
			},
		}
		external := &mockConsumer{} // UniFi-style consumer
		internal := &mockInternalConsumer{source: "ffmpeg:driveway"}
		s.consumers = append(s.consumers, external, internal)

		s.kickConsumers("test")

		require.Equal(t, int32(1), external.stops.Load(),
			"external consumer should be kicked")
		require.Equal(t, int32(0), internal.mockConsumer.stops.Load(),
			"internal-loop consumer must NOT be kicked")
	})

	t.Run("grace period not held when only internal consumers", func(t *testing.T) {
		// If every consumer is filtered out as internal, no kick happens
		// and we should not bump pending unnecessarily.
		s := &Stream{
			producers: []*Producer{{url: "ffmpeg:driveway"}},
		}
		internal := &mockInternalConsumer{source: "ffmpeg:driveway"}
		s.consumers = append(s.consumers, internal)

		s.kickConsumers("test")

		require.Equal(t, int32(0), s.pending.Load(),
			"pending should not be bumped when no consumers are kicked")
	})

	t.Run("grace period holds producers alive then releases", func(t *testing.T) {
		// Shorten the grace period so the test runs fast. The contract
		// being verified: pending counter is bumped synchronously by
		// kickConsumers, then decremented after the grace window.
		// stopProducers checks pending and short-circuits while it's
		// non-zero, so producers stay alive during the window.
		original := kickGracePeriod
		kickGracePeriod = 50 * time.Millisecond
		defer func() { kickGracePeriod = original }()

		s := &Stream{}
		s.consumers = append(s.consumers, &mockConsumer{})

		require.Equal(t, int32(0), s.pending.Load(),
			"pending should start at 0")

		s.kickConsumers("test")

		// Immediately after the kick, pending should be 1 — producers
		// are protected from premature stopProducers.
		require.Equal(t, int32(1), s.pending.Load(),
			"pending should be 1 during grace period")

		// After the grace window expires, pending returns to 0.
		require.Eventually(t,
			func() bool { return s.pending.Load() == 0 },
			500*time.Millisecond, 10*time.Millisecond,
			"pending should return to 0 after grace period")
	})
}

// TestRedactSourceURL verifies the helper used to scrub query strings
// from source URLs before they appear in log lines. Producer URLs like
// `nest:?client_id=…&client_secret=…` carry credentials that must not
// leak when debugging output is shared.
func TestRedactSourceURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"nest:?client_id=abc&client_secret=xyz&refresh_token=tok", "nest:"},
		{"rtsp://user:pass@host/stream?audio=copy", "rtsp://user:pass@host/stream"},
		{"rtsp://192.168.1.10/stream", "rtsp://192.168.1.10/stream"},
		{"ffmpeg:driveway#audio=aac", "ffmpeg:driveway"},
		{"ffmpeg:driveway?something#audio=aac", "ffmpeg:driveway"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			require.Equal(t, tt.want, redactSourceURL(tt.in))
		})
	}
}

// TestNewStreamLinksProducers verifies that NewStream sets the
// Producer.stream back-reference, which is required for the
// reconnect → kickConsumers wiring to work.
func TestNewStreamLinksProducers(t *testing.T) {
	t.Run("single string source", func(t *testing.T) {
		s := NewStream("rtsp://example.com")
		require.Len(t, s.producers, 1)
		require.Same(t, s, s.producers[0].stream,
			"producer.stream should point back to its parent Stream")
	})

	t.Run("multiple string sources", func(t *testing.T) {
		s := NewStream([]string{"rtsp://a", "rtsp://b", "rtsp://c"})
		require.Len(t, s.producers, 3)
		for i, p := range s.producers {
			require.Same(t, s, p.stream,
				"producer %d stream back-ref not set", i)
		}
	})

	t.Run("[]any sources", func(t *testing.T) {
		s := NewStream([]any{"rtsp://a", "rtsp://b"})
		require.Len(t, s.producers, 2)
		for i, p := range s.producers {
			require.Same(t, s, p.stream,
				"producer %d stream back-ref not set", i)
		}
	})

	t.Run("presentation stream control source", func(t *testing.T) {
		s := NewStream(map[string]any{
			"url":     "ffmpeg:rtsp://127.0.0.1:8554/porton#video=copy#audio=aac",
			"control": "porton",
		})
		require.Equal(t, "porton", s.control)
		require.Len(t, s.producers, 1)
		require.Equal(t, "ffmpeg:rtsp://127.0.0.1:8554/porton#video=copy#audio=aac", s.producers[0].url)
	})
}
