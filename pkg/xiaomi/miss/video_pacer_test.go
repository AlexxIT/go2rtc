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
