// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestHKSVSession creates a test hksvSession with connected HDS pairs.
// Returns the session, controller-side HDS session, and the server.
func newTestHKSVSession(t *testing.T, streams *mockStreamProvider) (*hksvSession, *hds.Session, *Server) {
	t.Helper()

	if streams == nil {
		streams = newMockStreamProvider()
	}
	srv := newTestServer(t, func(c *Config) {
		c.Streams = streams
	})

	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	accConn, err := hds.NewConn(c1, key, salt, false)
	require.NoError(t, err)
	ctrlConn, err := hds.NewConn(c2, key, salt, true)
	require.NoError(t, err)

	ctrl := hds.NewSession(ctrlConn)

	// nil hapConn is fine — handleOpen/handleClose don't use it
	hs := newHKSVSession(srv, nil, accConn)

	return hs, ctrl, srv
}

// ====================================================================
// handleOpen
// ====================================================================

func TestSession_HandleOpen_CreatesConsumer(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, srv := newTestHKSVSession(t, streams)

	// Drain controller side messages
	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	err := hs.handleOpen(1)
	require.NoError(t, err)

	// Consumer should be created and added to stream
	hs.mu.Lock()
	consumer := hs.consumer
	hs.mu.Unlock()
	require.NotNil(t, consumer)

	// Consumer should be added to stream provider
	require.Equal(t, 1, streams.count("test-camera"))

	// Consumer should be tracked in server connections
	srv.mu.Lock()
	require.Contains(t, srv.conns, consumer)
	srv.mu.Unlock()
}

func TestSession_HandleOpen_UsesPreparedConsumer(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, srv := newTestHKSVSession(t, streams)

	// Pre-prepare a consumer
	prepared := NewHKSVConsumer(zerolog.Nop())
	prepared.initData = []byte("fake-init")
	close(prepared.initDone)
	srv.preparedConsumer = prepared

	// Drain controller side
	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	err := hs.handleOpen(1)
	require.NoError(t, err)

	// Should use the prepared consumer
	hs.mu.Lock()
	consumer := hs.consumer
	hs.mu.Unlock()
	require.Equal(t, prepared, consumer)

	// preparedConsumer should be cleared
	require.Nil(t, srv.takePreparedConsumer())
}

func TestSession_HandleOpen_StreamError(t *testing.T) {
	streams := newMockStreamProvider()
	streams.addErr = errors.New("stream offline")
	hs, _, _ := newTestHKSVSession(t, streams)

	err := hs.handleOpen(1)
	require.NoError(t, err) // handleOpen returns nil even on error

	hs.mu.Lock()
	require.Nil(t, hs.consumer, "consumer should not be set on stream error")
	hs.mu.Unlock()
}

func TestSession_HandleOpen_ReplacesExistingConsumer(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, _ := newTestHKSVSession(t, streams)

	// Drain controller side
	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// First open
	_ = hs.handleOpen(1)
	hs.mu.Lock()
	first := hs.consumer
	hs.mu.Unlock()
	require.NotNil(t, first)

	// Second open should stop the first consumer
	_ = hs.handleOpen(2)
	hs.mu.Lock()
	second := hs.consumer
	hs.mu.Unlock()
	require.NotNil(t, second)
	require.NotEqual(t, first, second)

	// First consumer should be stopped
	select {
	case <-first.Done():
		// OK
	default:
		t.Fatal("first consumer should be stopped when replaced")
	}
}

// ====================================================================
// handleClose
// ====================================================================

func TestSession_HandleClose_StopsRecording(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, srv := newTestHKSVSession(t, streams)

	// Drain controller
	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	_ = hs.handleOpen(1)
	hs.mu.Lock()
	consumer := hs.consumer
	hs.mu.Unlock()
	require.NotNil(t, consumer)

	_ = hs.handleClose(1)

	// Consumer should be stopped and removed
	hs.mu.Lock()
	require.Nil(t, hs.consumer)
	hs.mu.Unlock()

	select {
	case <-consumer.Done():
	default:
		t.Fatal("consumer should be stopped after handleClose")
	}

	require.Equal(t, 0, streams.count("test-camera"))

	srv.mu.Lock()
	require.NotContains(t, srv.conns, consumer)
	srv.mu.Unlock()
}

func TestSession_HandleClose_NoConsumer(t *testing.T) {
	hs, _, _ := newTestHKSVSession(t, nil)
	// Should not panic when no consumer
	err := hs.handleClose(1)
	require.NoError(t, err)
}

// ====================================================================
// Close
// ====================================================================

func TestSession_Close_StopsActiveRecording(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, _ := newTestHKSVSession(t, streams)

	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	_ = hs.handleOpen(1)
	hs.mu.Lock()
	consumer := hs.consumer
	hs.mu.Unlock()

	hs.Close()

	select {
	case <-consumer.Done():
	default:
		t.Fatal("Close should stop active consumer")
	}
}

func TestSession_Close_NoActiveRecording(t *testing.T) {
	hs, _, _ := newTestHKSVSession(t, nil)
	// Should not panic
	hs.Close()
}

// ====================================================================
// Full Session Lifecycle (integration)
// ====================================================================

func TestSession_FullLifecycle(t *testing.T) {
	// Simulates: open → stream → close → re-open → close

	streams := newMockStreamProvider()
	hs, ctrl, srv := newTestHKSVSession(t, streams)

	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	// First recording session
	_ = hs.handleOpen(1)
	hs.mu.Lock()
	c1 := hs.consumer
	hs.mu.Unlock()
	require.NotNil(t, c1)
	require.Equal(t, 1, streams.count("test-camera"))

	// End first recording
	_ = hs.handleClose(1)
	require.Equal(t, 0, streams.count("test-camera"))

	// Second recording session (re-open)
	_ = hs.handleOpen(2)
	hs.mu.Lock()
	c2 := hs.consumer
	hs.mu.Unlock()
	require.NotNil(t, c2)
	require.NotEqual(t, c1, c2, "should be a new consumer")
	require.Equal(t, 1, streams.count("test-camera"))

	// Final close
	hs.Close()
	require.Equal(t, 0, streams.count("test-camera"))

	// Verify server cleanup
	srv.mu.Lock()
	require.Empty(t, srv.conns)
	srv.mu.Unlock()
}

// ====================================================================
// stopRecording
// ====================================================================

func TestStopRecording_FullCleanup(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, srv := newTestHKSVSession(t, streams)

	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	_ = hs.handleOpen(1)
	hs.mu.Lock()
	consumer := hs.consumer
	hs.mu.Unlock()

	// Verify consumer is tracked
	srv.mu.Lock()
	require.Contains(t, srv.conns, consumer)
	srv.mu.Unlock()
	require.Equal(t, 1, streams.count("test-camera"))

	// Stop recording
	hs.mu.Lock()
	hs.stopRecording()
	hs.mu.Unlock()

	// Verify full cleanup
	hs.mu.Lock()
	require.Nil(t, hs.consumer)
	hs.mu.Unlock()

	select {
	case <-consumer.Done():
	default:
		t.Fatal("consumer should be stopped")
	}

	require.Equal(t, 0, streams.count("test-camera"))

	srv.mu.Lock()
	require.NotContains(t, srv.conns, consumer)
	srv.mu.Unlock()
}

// ====================================================================
// Concurrent Session Operations
// ====================================================================

func TestSession_ConcurrentOpenClose(t *testing.T) {
	streams := newMockStreamProvider()
	hs, ctrl, _ := newTestHKSVSession(t, streams)

	go func() {
		for {
			if _, err := ctrl.ReadMessage(); err != nil {
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				_ = hs.handleOpen(n)
			} else {
				_ = hs.handleClose(n)
			}
		}(i)
	}
	wg.Wait()

	// Clean close at the end
	hs.Close()

	// Verify no leaked consumers
	require.Eventually(t, func() bool {
		return streams.count("test-camera") == 0
	}, 2*time.Second, 50*time.Millisecond)
}

// ====================================================================
// Server acceptHDS integration (partial)
// ====================================================================

func TestServer_AcceptHDS_Lifecycle(t *testing.T) {
	// Test the session stored in server is properly managed

	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.Streams = streams
	})

	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	defer c2.Close()

	accConn, err := hds.NewConn(c1, key, salt, false)
	require.NoError(t, err)

	hs := newHKSVSession(srv, nil, accConn)

	srv.mu.Lock()
	srv.hksvSession = hs
	srv.mu.Unlock()

	// Verify session is set
	srv.mu.Lock()
	require.NotNil(t, srv.hksvSession)
	srv.mu.Unlock()

	// Cleanup: session removal
	srv.mu.Lock()
	if srv.hksvSession == hs {
		srv.hksvSession = nil
	}
	srv.mu.Unlock()
	hs.Close()

	srv.mu.Lock()
	require.Nil(t, srv.hksvSession)
	srv.mu.Unlock()
}

// ====================================================================
// prepareHKSVConsumer integration
// ====================================================================

func TestPrepareHKSVConsumer_Flow(t *testing.T) {
	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.MotionMode = "continuous"
		c.Streams = streams
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.prepareHKSVConsumer()
	}()

	// Wait for consumer to be prepared
	require.Eventually(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return srv.preparedConsumer != nil
	}, 2*time.Second, 10*time.Millisecond)

	// Take the prepared consumer
	consumer := srv.takePreparedConsumer()
	require.NotNil(t, consumer)

	// Stop it (this triggers done channel → goroutine exits)
	_ = consumer.Stop()
	<-done
}

func TestPrepareHKSVConsumer_StreamError(t *testing.T) {
	streams := newMockStreamProvider()
	streams.addErr = errors.New("no stream")
	srv := newTestServer(t, func(c *Config) {
		c.Streams = streams
	})

	srv.prepareHKSVConsumer()

	require.Nil(t, srv.preparedConsumer)
}

func TestPrepareHKSVConsumer_ReplacesOld(t *testing.T) {
	streams := newMockStreamProvider()
	srv := newTestServer(t, func(c *Config) {
		c.Streams = streams
	})

	// Start first prepare
	done1 := make(chan struct{})
	go func() {
		defer close(done1)
		srv.prepareHKSVConsumer()
	}()

	require.Eventually(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return srv.preparedConsumer != nil
	}, 2*time.Second, 10*time.Millisecond)

	srv.mu.Lock()
	first := srv.preparedConsumer
	srv.mu.Unlock()

	// Start second prepare — should replace the first
	done2 := make(chan struct{})
	go func() {
		defer close(done2)
		srv.prepareHKSVConsumer()
	}()

	// Wait for replacement
	require.Eventually(t, func() bool {
		srv.mu.Lock()
		defer srv.mu.Unlock()
		return srv.preparedConsumer != nil && srv.preparedConsumer != first
	}, 2*time.Second, 10*time.Millisecond)

	// First consumer should be stopped
	select {
	case <-first.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("first consumer should be stopped")
	}

	<-done1

	// Clean up
	srv.mu.Lock()
	c := srv.preparedConsumer
	srv.mu.Unlock()
	if c != nil {
		_ = c.Stop()
	}
	<-done2
}

// ====================================================================
// Benchmarks
// ====================================================================

func BenchmarkServer_AddDelConn(b *testing.B) {
	streams := newMockStreamProvider()
	srv, _ := NewServer(Config{
		StreamName: "bench",
		Pin:        "27041991",
		HKSV:       true,
		Streams:    streams,
		Logger:     zerolog.Nop(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := i
		srv.AddConn(conn)
		srv.DelConn(conn)
	}
}

func BenchmarkServer_AddDelPair(b *testing.B) {
	streams := newMockStreamProvider()
	srv, _ := NewServer(Config{
		StreamName: "bench",
		Pin:        "27041991",
		HKSV:       true,
		Streams:    streams,
		Logger:     zerolog.Nop(),
	})

	pub := []byte{1, 2, 3, 4}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := assert.AnError.Error() // just a string
		srv.AddPair(id, pub, hap.PermissionAdmin)
		srv.DelPair(id)
	}
}

func BenchmarkServer_SetMotionDetected(b *testing.B) {
	streams := newMockStreamProvider()
	srv, _ := NewServer(Config{
		StreamName: "bench",
		Pin:        "27041991",
		HKSV:       true,
		Streams:    streams,
		Logger:     zerolog.Nop(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		srv.SetMotionDetected(i%2 == 0)
	}
}

func BenchmarkServer_MarshalJSON(b *testing.B) {
	streams := newMockStreamProvider()
	srv, _ := NewServer(Config{
		StreamName: "bench",
		Pin:        "27041991",
		HKSV:       true,
		Streams:    streams,
		Logger:     zerolog.Nop(),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = srv.MarshalJSON()
	}
}
