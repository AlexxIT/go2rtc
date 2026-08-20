package dahua

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestDHAVAudioCodecID(t *testing.T) {
	for _, tc := range []struct {
		codec string
		want  byte
	}{
		{core.CodecPCMA, 0x0E}, // G.711A
		{core.CodecPCMU, 0x0A}, // G.711Mu
		{core.CodecPCML, 0x0C}, // PCM_S16LE
		{core.CodecAAC, 0x1A},
	} {
		got, ok := dhavAudioCodecID(tc.codec)
		require.True(t, ok, tc.codec)
		require.Equal(t, tc.want, got, tc.codec)
	}

	_, ok := dhavAudioCodecID(core.CodecOpus)
	require.False(t, ok, "opus is not a Dahua talk codec")
}

func TestDHAVSampleRateIndex(t *testing.T) {
	// 8000 must map to 2, not 0: both are 8 kHz in the table but Dahua's own
	// client emits 2 and some firmware rejects 0.
	require.Equal(t, byte(2), dhavSampleRateIndex(8000))
	require.Equal(t, byte(4), dhavSampleRateIndex(16000))
	require.Equal(t, byte(8), dhavSampleRateIndex(44100))
	require.Equal(t, byte(2), dhavSampleRateIndex(12345), "unknown rate falls back to 8 kHz")
}

func TestFrameBytes(t *testing.T) {
	// 40 ms of 8 kHz G.711 = 320 bytes, the chunk size Dahua clients use.
	require.Equal(t, 320, frameBytes(&core.Codec{Name: core.CodecPCMA, ClockRate: 8000}, 40))
	require.Equal(t, 640, frameBytes(&core.Codec{Name: core.CodecPCML, ClockRate: 8000}, 40))
	require.Equal(t, 0, frameBytes(&core.Codec{Name: core.CodecAAC, ClockRate: 8000}, 40),
		"compressed codecs keep their native frame size")
}
