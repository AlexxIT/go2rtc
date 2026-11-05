package mpegts

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClock(t *testing.T) {
	// 320 samples of 8000 Hz = 40 ms = 3600 ticks of 90000 Hz
	for _, start := range []uint32{0, 1000, 381_000_000, 2_200_000_000, 4294960000} {
		c := NewClock(8000)
		ts := start
		prev := c.Convert(ts)
		// 4294960000 + 320*100 crosses the uint32 wraparound
		for i := 0; i < 100; i++ {
			ts += 320
			pts := c.Convert(ts)
			require.Equal(t, uint32(3600), pts-prev, "start=%d step=%d", start, i)
			prev = pts
		}
	}

	// AAC 44100 Hz, 1024 samples
	c := NewClock(44100)
	ts := uint32(4294960000)
	prev := c.Convert(ts)
	var total uint64
	for i := 0; i < 1000; i++ {
		ts += 1024
		pts := c.Convert(ts)
		total += uint64(pts - prev)
		prev = pts
	}
	require.InDelta(t, 1000*1024*90000/44100, int(total), 1000)
}

func TestClockNoDrift(t *testing.T) {
	// AAC 44100 Hz: 1024 samples is 2089.79 ticks at 90 kHz, so per-step rounding would drift
	c := NewClock(44100)
	ts := uint32(4294900000)
	first := c.Convert(ts)
	const frames = 44100 // 1024 s of audio, crosses the uint32 wraparound
	var pts uint32
	for i := 0; i < frames; i++ {
		ts += 1024
		pts = c.Convert(ts)
	}
	want := uint32(uint64(frames) * 1024 * 90000 / 44100)
	if got := pts - first; got != want {
		t.Fatalf("after %d frames PTS moved %d, want %d", frames, got, want)
	}
}
