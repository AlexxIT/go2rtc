package miss

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const opusFrame = uint32(timestamp40ms)

func TestAudioClockSmoothUnderJitter(t *testing.T) {
	var c audioClock
	cameraMS := []uint64{1000, 1032, 1084, 1118, 1160, 1197} // ~40ms with jitter
	first := c.Next(cameraMS[0], 48000, opusFrame)
	for i, ms := range cameraMS[1:] {
		require.Equal(t, first+uint32(i+1)*opusFrame, c.Next(ms, 48000, opusFrame))
	}
}

func TestAudioClockResyncsAfterStall(t *testing.T) {
	var c audioClock
	c.Next(1000, 48000, opusFrame)
	c.Next(1040, 48000, opusFrame)
	// 15s of packets lost: the counter alone would say 1080ms
	require.Equal(t, uint32(16080*48), c.Next(16080, 48000, opusFrame))
	require.Equal(t, uint32(16080*48)+opusFrame, c.Next(16120, 48000, opusFrame))
}

func TestAudioClockWithoutCameraTimestamp(t *testing.T) {
	var c audioClock
	require.Equal(t, uint32(0), c.Next(0, 8000, 160))
	require.Equal(t, uint32(160), c.Next(0, 8000, 160))
}
