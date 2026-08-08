package miss

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRTPDuration(t *testing.T) {
	require.Equal(t, 100*time.Millisecond, rtpDuration(9000))
	require.Equal(t, time.Duration(0), rtpDuration(videoClockRate*61))
}

func TestPacedFrameInterval(t *testing.T) {
	buffer := 400 * time.Millisecond
	interval := 100 * time.Millisecond

	require.Equal(t, interval, pacedFrameInterval(interval, 400*time.Millisecond, buffer))
	require.Equal(t, 75*time.Millisecond, pacedFrameInterval(interval, 700*time.Millisecond, buffer))
	require.Equal(t, 50*time.Millisecond, pacedFrameInterval(interval, 1100*time.Millisecond, buffer))
}

func TestPacedReleaseAnchorDoesNotAccumulateTimerOvershoot(t *testing.T) {
	interval := 50 * time.Millisecond
	start := time.Unix(100, 0)
	anchor := start

	for range 12000 { // ten minutes at 20 fps
		actual := anchor.Add(interval + time.Millisecond)
		anchor = pacedReleaseAnchor(anchor, actual, interval)
	}

	require.Equal(t, start.Add(10*time.Minute), anchor)
}

func TestPacedReleaseAnchorRebasesAfterRealStall(t *testing.T) {
	interval := 50 * time.Millisecond
	last := time.Unix(100, 0)
	actual := last.Add(2*interval + time.Nanosecond)

	require.Equal(t, actual, pacedReleaseAnchor(last, actual, interval))
}
