package homekit

import (
	"sync"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// SILK NB, 20 ms, 1 frame (as sent by Logitech Circle View)
var opus20ms = []byte{0x08, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func newTestClock() *testClock {
	return &testClock{t: time.Unix(1_000_000, 0)}
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func collect(timestamps *[]uint32) core.HandlerFunc {
	return func(packet *rtp.Packet) {
		*timestamps = append(*timestamps, packet.Timestamp)
	}
}

func TestOpusDuration(t *testing.T) {
	require.Equal(t, 20*time.Millisecond, opusDuration([]byte{0x08, 1}))       // NB 20 ms, 1 frame
	require.Equal(t, 40*time.Millisecond, opusDuration([]byte{0x09, 1}))       // NB 20 ms, 2 frames (code 1)
	require.Equal(t, 60*time.Millisecond, opusDuration([]byte{0x0B, 0x03, 1})) // NB 20 ms, code 3, 3 frames
	require.Equal(t, 60*time.Millisecond, opusDuration([]byte{0x18, 1}))       // NB 60 ms, 1 frame
	require.Equal(t, 10*time.Millisecond, opusDuration([]byte{0x40, 1}))       // WB 10 ms, 1 frame
	require.Equal(t, 120*time.Millisecond, opusDuration([]byte{0xE3, 48, 1}))  // FB 2.5 ms (config 28), code 3, 48 frames

	require.Zero(t, opusDuration(nil))                   // empty
	require.Zero(t, opusDuration([]byte{0x0B}))          // code 3 without frame count
	require.Zero(t, opusDuration([]byte{0x0B, 0x00}))    // code 3, 0 frames
	require.Zero(t, opusDuration([]byte{0x0B, 49}))      // code 3, more than 48 frames
	require.Zero(t, opusDuration([]byte{0x0B, 0x07, 1})) // 7 x 20 ms > 120 ms
}

func TestTimekeeperOpusRealtime(t *testing.T) {
	clock := newTestClock()

	var got []uint32
	handler := timekeeperOpus(collect(&got), 8000, clock.now)

	for i := 0; i < 50; i++ {
		handler(&rtp.Packet{Payload: opus20ms})
		clock.add(20 * time.Millisecond)
	}

	require.Len(t, got, 50) // no drops at 50 packets/s (timekeeper keeps only 2/3)
	for i, ts := range got {
		require.Equal(t, uint32(i*160), ts) // 20 ms @ 8 kHz
	}
}

func TestTimekeeperOpusBurst(t *testing.T) {
	clock := newTestClock()

	var got []uint32
	handler := timekeeperOpus(collect(&got), 8000, clock.now)

	for i := 0; i < 30; i++ {
		handler(&rtp.Packet{Payload: opus20ms}) // all at the same instant
	}

	// accepted while the sent audio is not more than 250 ms ahead of real time
	require.Len(t, got, 13)
}

func TestTimekeeperOpusGap(t *testing.T) {
	clock := newTestClock()

	var got []uint32
	handler := timekeeperOpus(collect(&got), 16000, clock.now)

	for i := 0; i < 5; i++ {
		handler(&rtp.Packet{Payload: opus20ms})
		clock.add(20 * time.Millisecond)
	}
	clock.add(3 * time.Second) // camera audio pause
	handler(&rtp.Packet{Payload: opus20ms})

	require.Len(t, got, 6)
	require.Equal(t, uint32(4*320), got[4])
	// next timestamp jumps to real time: 100 ms of audio + 3 s gap @ 16 kHz
	require.Equal(t, uint32((100+3000)*16), got[5])
}

func TestTimekeeperOpusLost(t *testing.T) {
	clock := newTestClock()

	var got []uint32
	handler := timekeeperOpus(collect(&got), 8000, clock.now)

	for _, seq := range []uint16{65534, 65535, 0, 3, 4, 1000} { // 1 and 2 lost, then a big jump
		handler(&rtp.Packet{Header: rtp.Header{SequenceNumber: seq}, Payload: opus20ms})
		clock.add(20 * time.Millisecond)
		if seq == 0 {
			clock.add(40 * time.Millisecond) // time of the lost packets
		}
	}

	// 2 lost packets keep the real time (320 + 160 + 2*160), a big jump in sequence is not counted as loss
	require.Equal(t, []uint32{0, 160, 320, 800, 960, 1120}, got)
}

func TestTimekeeperOpusInvalid(t *testing.T) {
	clock := newTestClock()

	var got []uint32
	handler := timekeeperOpus(collect(&got), 8000, clock.now)

	handler(&rtp.Packet{})                            // empty payload: dropped
	handler(&rtp.Packet{Payload: []byte{0x0B, 0x00}}) // invalid TOC: requested 20 ms
	clock.add(20 * time.Millisecond)
	handler(&rtp.Packet{Payload: opus20ms})

	require.Equal(t, []uint32{0, 160}, got)
}

func TestTimekeeperLegacy(t *testing.T) {
	var got []uint32
	handler := timekeeper(collect(&got))

	handler(&rtp.Packet{Payload: []byte{1}})
	require.Equal(t, []uint32{480}, got) // unchanged behavior for non-Opus codecs
}
