package h264

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/stretchr/testify/require"
)

func TestIsKeyframeRTP(t *testing.T) {
	pkt := func(b ...byte) *core.Packet { return &core.Packet{Payload: b} }

	require.True(t, IsKeyframeRTP(pkt(0x67, 0x42)))                                           // SPS
	require.True(t, IsKeyframeRTP(pkt(0x65, 0x88)))                                           // IDR slice, unfragmented
	require.False(t, IsKeyframeRTP(pkt(0x41, 0x9a)))                                          // P slice
	require.False(t, IsKeyframeRTP(pkt(0x68, 0xce)))                                          // PPS alone
	require.True(t, IsKeyframeRTP(pkt(0x18, 0x00, 0x02, 0x67, 0x42, 0x00, 0x02, 0x68, 0xce))) // STAP-A: SPS, PPS
	require.False(t, IsKeyframeRTP(pkt(0x18, 0x00, 0x02, 0x06, 0x05)))                        // STAP-A: SEI only
	require.True(t, IsKeyframeRTP(pkt(0x7c, 0x85, 0x88)))                                     // FU-A, IDR, start
	require.False(t, IsKeyframeRTP(pkt(0x7c, 0x05, 0x88)))                                    // FU-A, IDR, continuation
	require.False(t, IsKeyframeRTP(pkt(0x7c, 0x81, 0x9a)))                                    // FU-A, P slice, start
	require.False(t, IsKeyframeRTP(pkt(0x18, 0x00, 0x09, 0x67)))                              // STAP-A, truncated
}
