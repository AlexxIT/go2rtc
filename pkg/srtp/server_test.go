package srtp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSSRC(t *testing.T) {
	// RTP with any payload type, with and without marker
	for _, pt := range []byte{0, 8, 96, 99, 110, 111, 0x80 | 110, 0x80 | 111} {
		b := []byte{0x80, pt, 0, 1, 0, 0, 0, 0, 0x12, 0x34, 0x56, 0x78}
		rtcp, ssrc, ok := parseSSRC(b)
		require.True(t, ok, pt)
		require.False(t, rtcp, pt)
		require.Equal(t, uint32(0x12345678), ssrc, pt)
	}

	// RTCP sender report
	b := []byte{0x80, 200, 0, 6, 0x87, 0x65, 0x43, 0x21, 0, 0, 0, 0}
	rtcp, ssrc, ok := parseSSRC(b)
	require.True(t, ok)
	require.True(t, rtcp)
	require.Equal(t, uint32(0x87654321), ssrc)

	// too short for an RTP header
	_, _, ok = parseSSRC([]byte{0x80, 110, 0, 1})
	require.False(t, ok)
}
