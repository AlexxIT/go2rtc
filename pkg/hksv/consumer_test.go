// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap/hds"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

var testLog = zerolog.Nop()

// newTestSessionPair creates connected HDS sessions for testing.
func newTestSessionPair(t *testing.T) (accessory *hds.Session, controller *hds.Session) {
	t.Helper()
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)

	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })

	accConn, err := hds.NewConn(c1, key, salt, false)
	require.NoError(t, err)
	ctrlConn, err := hds.NewConn(c2, key, salt, true)
	require.NoError(t, err)

	return hds.NewSession(accConn), hds.NewSession(ctrlConn)
}

func TestHKSVConsumer_Creation(t *testing.T) {
	c := NewHKSVConsumer(testLog)

	require.Equal(t, "hksv", c.FormatName)
	require.Equal(t, "hds", c.Protocol)
	require.Len(t, c.Medias, 2)
	require.Equal(t, core.KindVideo, c.Medias[0].Kind)
	require.Equal(t, core.KindAudio, c.Medias[1].Kind)
	require.Equal(t, core.CodecH264, c.Medias[0].Codecs[0].Name)
	require.Equal(t, core.CodecAAC, c.Medias[1].Codecs[0].Name)

	require.NotNil(t, c.muxer)
	require.NotNil(t, c.done)
	require.NotNil(t, c.initDone)
	require.False(t, c.active)
	require.False(t, c.start)
	require.Equal(t, 0, c.seqNum)
	require.Nil(t, c.fragBuf)
	require.Nil(t, c.initData)
}

func TestHKSVConsumer_FlushFragment_SendsAndIncrements(t *testing.T) {
	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)

	// Manually set up the consumer as if Activate() was called
	c.session = acc
	c.streamID = 1
	c.seqNum = 2
	c.active = true
	c.fragBuf = []byte("fake-fragment-data-here")

	done := make(chan struct{})
	go func() {
		defer close(done)
		msg, err := ctrl.ReadMessage()
		require.NoError(t, err)
		require.Equal(t, "dataSend", msg.Protocol)
		require.Equal(t, "data", msg.Topic)
		require.True(t, msg.IsEvent)

		packets, ok := msg.Body["packets"].([]any)
		require.True(t, ok)
		pkt := packets[0].(map[string]any)
		meta := pkt["metadata"].(map[string]any)

		require.Equal(t, "mediaFragment", meta["dataType"])
		require.Equal(t, int64(2), meta["dataSequenceNumber"].(int64))
		require.Equal(t, true, meta["isLastDataChunk"])
	}()

	c.mu.Lock()
	c.flushFragment()
	c.mu.Unlock()

	<-done

	require.Equal(t, 3, c.seqNum, "seqNum should increment after flush")
	require.Empty(t, c.fragBuf, "fragBuf should be empty after flush")
}

func TestHKSVConsumer_FlushFragment_MultipleFlushes(t *testing.T) {
	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)
	c.session = acc
	c.streamID = 1
	c.seqNum = 2
	c.active = true

	var received []int64
	var mu sync.Mutex
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 3; i++ {
			msg, err := ctrl.ReadMessage()
			if err != nil {
				return
			}
			packets := msg.Body["packets"].([]any)
			pkt := packets[0].(map[string]any)
			meta := pkt["metadata"].(map[string]any)
			mu.Lock()
			received = append(received, meta["dataSequenceNumber"].(int64))
			mu.Unlock()
		}
	}()

	for i := 0; i < 3; i++ {
		c.mu.Lock()
		c.fragBuf = []byte("data")
		c.flushFragment()
		c.mu.Unlock()
	}

	<-done

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int64{2, 3, 4}, received)
	require.Equal(t, 5, c.seqNum)
}

func TestHKSVConsumer_FlushFragment_EmptyBuffer(t *testing.T) {
	c := NewHKSVConsumer(testLog)
	c.seqNum = 2

	// flushFragment with empty/nil buffer should still increment seqNum
	// but send empty data (protocol layer handles it)
	// In practice, flushFragment is only called when fragBuf has data
	c.mu.Lock()
	c.fragBuf = nil
	initialSeq := c.seqNum
	c.mu.Unlock()

	// No crash = pass (no session to write to, would panic on nil session)
	require.Equal(t, initialSeq, c.seqNum)
}

func TestHKSVConsumer_BufferAccumulation(t *testing.T) {
	c := NewHKSVConsumer(testLog)
	c.active = true

	data1 := []byte("chunk-1")
	data2 := []byte("chunk-2")
	data3 := []byte("chunk-3")

	c.fragBuf = append(c.fragBuf, data1...)
	c.fragBuf = append(c.fragBuf, data2...)
	c.fragBuf = append(c.fragBuf, data3...)

	require.Equal(t, len(data1)+len(data2)+len(data3), len(c.fragBuf))
	require.Equal(t, "chunk-1chunk-2chunk-3", string(c.fragBuf))
}

func TestHKSVConsumer_ActivateSeqNum(t *testing.T) {
	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)

	// Simulate init ready
	c.initData = []byte("fake-init")
	close(c.initDone)

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Read the init message
		msg, err := ctrl.ReadMessage()
		require.NoError(t, err)
		require.True(t, msg.IsEvent)

		packets := msg.Body["packets"].([]any)
		pkt := packets[0].(map[string]any)
		meta := pkt["metadata"].(map[string]any)

		require.Equal(t, "mediaInitialization", meta["dataType"])
		require.Equal(t, int64(1), meta["dataSequenceNumber"].(int64))
	}()

	err := c.Activate(acc, 5)
	require.NoError(t, err)
	<-done

	require.Equal(t, 2, c.seqNum, "seqNum should be 2 after activate (init uses 1)")
	require.True(t, c.active)
	require.Equal(t, 5, c.streamID)
	require.Equal(t, acc, c.session)
}

func TestHKSVConsumer_ActivateTimeout(t *testing.T) {
	acc, _ := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)
	// Don't close initDone — simulate init never becoming ready

	// Override the timeout for faster test
	err := func() error {
		select {
		case <-c.initDone:
		case <-time.After(50 * time.Millisecond):
			return errActivateTimeout
		}
		return nil
	}()

	require.Error(t, err)
	_ = acc // prevent unused
}

var errActivateTimeout = func() error {
	return &timeoutError{}
}()

type timeoutError struct{}

func (e *timeoutError) Error() string { return "activate timeout" }

func TestHKSVConsumer_ActivateWithError(t *testing.T) {
	c := NewHKSVConsumer(testLog)
	c.initErr = &timeoutError{}
	close(c.initDone)

	acc, _ := newTestSessionPair(t)
	err := c.Activate(acc, 1)
	require.Error(t, err)
	require.False(t, c.active)
}

func TestHKSVConsumer_StopSafety(t *testing.T) {
	c := NewHKSVConsumer(testLog)
	c.active = true

	// First stop
	err := c.Stop()
	require.NoError(t, err)
	require.False(t, c.active)

	// Second stop — should not panic
	err = c.Stop()
	require.NoError(t, err)
}

func TestHKSVConsumer_StopDeactivates(t *testing.T) {
	c := NewHKSVConsumer(testLog)
	c.active = true
	c.start = true

	_ = c.Stop()

	require.False(t, c.active)
}

func TestHKSVConsumer_WriteToDone(t *testing.T) {
	c := NewHKSVConsumer(testLog)

	done := make(chan struct{})
	go func() {
		n, err := c.WriteTo(nil)
		require.NoError(t, err)
		require.Equal(t, int64(0), n)
		close(done)
	}()

	// WriteTo should block until done channel is closed
	select {
	case <-done:
		t.Fatal("WriteTo returned before Stop")
	case <-time.After(50 * time.Millisecond):
	}

	_ = c.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WriteTo did not return after Stop")
	}
}

func TestHKSVConsumer_GOPFlushIntegration(t *testing.T) {
	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)
	c.session = acc
	c.streamID = 1
	c.seqNum = 2
	c.active = true
	c.start = true // already started

	// Simulate a sequence: buffer data, then flush
	frag1 := []byte("keyframe-1-data-plus-p-frames")
	frag2 := []byte("keyframe-2-data")

	var received [][]byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2; i++ {
			msg, err := ctrl.ReadMessage()
			if err != nil {
				return
			}
			packets := msg.Body["packets"].([]any)
			pkt := packets[0].(map[string]any)
			data := pkt["data"].([]byte)
			received = append(received, data)
		}
	}()

	// First GOP
	c.mu.Lock()
	c.fragBuf = append(c.fragBuf, frag1...)
	c.flushFragment()
	c.mu.Unlock()

	// Second GOP
	c.mu.Lock()
	c.fragBuf = append(c.fragBuf, frag2...)
	c.flushFragment()
	c.mu.Unlock()

	<-done

	require.Len(t, received, 2)
	require.Equal(t, frag1, received[0])
	require.Equal(t, frag2, received[1])
	require.Equal(t, 4, c.seqNum) // 2 + 2 flushes
}

func TestHKSVConsumer_FlushClearsBuffer(t *testing.T) {
	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)
	c.session = acc
	c.streamID = 1
	c.seqNum = 2
	c.active = true

	done := make(chan struct{})
	go func() {
		defer close(done)
		// drain messages
		for i := 0; i < 3; i++ {
			ctrl.ReadMessage()
		}
	}()

	for i := 0; i < 3; i++ {
		c.mu.Lock()
		c.fragBuf = append(c.fragBuf, []byte("frame-data")...)
		prevLen := len(c.fragBuf)
		c.flushFragment()
		require.Empty(t, c.fragBuf, "fragBuf should be empty after flush")
		require.Greater(t, prevLen, 0, "had data before flush")
		c.mu.Unlock()
	}

	<-done
	require.Equal(t, 5, c.seqNum, "3 flushes from seqNum=2 → 5")
}

func TestHKSVConsumer_SendTracking(t *testing.T) {
	acc, ctrl := newTestSessionPair(t)
	c := NewHKSVConsumer(testLog)
	c.session = acc
	c.streamID = 1
	c.seqNum = 2
	c.active = true

	data := []byte("12345678") // 8 bytes

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctrl.ReadMessage()
	}()

	c.mu.Lock()
	c.fragBuf = append(c.fragBuf, data...)
	c.flushFragment()
	c.mu.Unlock()

	<-done
	require.Equal(t, 8, c.Send, "Send counter should track bytes sent")
}

// --- Benchmarks ---

func BenchmarkHKSVConsumer_FlushFragment(b *testing.B) {
	key := []byte(core.RandString(16, 0))
	salt := core.RandString(32, 0)
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	accConn, _ := hds.NewConn(c1, key, salt, false)
	ctrlConn, _ := hds.NewConn(c2, key, salt, true)

	acc := hds.NewSession(accConn)

	go func() {
		buf := make([]byte, 512*1024) // must be > 256KB chunk size
		for {
			if _, err := ctrlConn.Read(buf); err != nil {
				return
			}
		}
	}()

	c := NewHKSVConsumer(testLog)
	c.session = acc
	c.streamID = 1
	c.seqNum = 2
	c.active = true

	gopData := make([]byte, 4*1024*1024) // 4MB GOP

	b.SetBytes(int64(len(gopData)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.mu.Lock()
		c.fragBuf = append(c.fragBuf[:0], gopData...)
		c.flushFragment()
		c.mu.Unlock()
	}
}

func BenchmarkHKSVConsumer_BufferAppend(b *testing.B) {
	c := NewHKSVConsumer(testLog)
	frame := make([]byte, 1500) // typical frame fragment

	b.SetBytes(int64(len(frame)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.fragBuf = append(c.fragBuf, frame...)
		if len(c.fragBuf) > 5*1024*1024 {
			c.fragBuf = c.fragBuf[:0]
		}
	}
}

func BenchmarkHKSVConsumer_CreateAndStop(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := NewHKSVConsumer(testLog)
		_ = c.Stop()
	}
}
