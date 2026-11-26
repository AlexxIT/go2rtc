package h265

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRepairAVCCDropsMalformedPayload(t *testing.T) {
	repair := RepairAVCC(&core.Codec{}, func(*rtp.Packet) {
		t.Fatal("forwarded a malformed AVCC packet")
	})

	for _, payload := range [][]byte{
		nil,
		{0, 0, 0},
		{0, 0, 0, 0},
		h264.JoinNALU([]byte{NALUTypeIFrame << 1}),
		{0, 0, 0, 3, NALUTypeIFrame << 1, 1},
	} {
		repair(&rtp.Packet{Payload: payload})
		require.False(t, IsKeyframe(payload))
	}
}

func TestRepairAVCCPreservesKeyframeWithEmptyNAL(t *testing.T) {
	idr := []byte{NALUTypeIFrame << 1, 1}
	payload := append(make([]byte, 4), h264.JoinNALU(idr)...)
	payload = append(payload, 0, 0, 0, 0)
	original := bytes.Clone(payload)
	// VPS: 40 01.
	codec := &core.Codec{FmtpLine: "sprop-vps=QAE="}
	input := &rtp.Packet{Header: rtp.Header{Timestamp: 123, Marker: true}, Payload: payload}
	var output *rtp.Packet
	repair := RepairAVCC(codec, func(packet *rtp.Packet) {
		output = packet
	})
	repair(input)

	require.True(t, IsKeyframe(input.Payload))
	require.Equal(t, original, input.Payload)
	require.NotNil(t, output)
	require.NotSame(t, input, output)
	require.Equal(t, input.Header, output.Header)
	require.Equal(t, h264.JoinNALU([]byte{0x40, 1}, idr), output.Payload)
}

func TestNALUTypeShortPayload(t *testing.T) {
	for size := range 4 + h265NaluHeaderSize {
		require.Equal(t, byte(0xFF), NALUType(make([]byte, size)))
	}
	require.Zero(t, NALUType(h264.JoinNALU([]byte{0, 1})))
}
