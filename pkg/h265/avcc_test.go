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

func TestAVCCToCodecDropsMalformedInputSafely(t *testing.T) {
	for _, avcc := range [][]byte{
		nil,
		{},
		{0},
		{0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 1, 0x40},       // one-byte NAL header
		{0, 0, 0, 2, 0x40},       // truncated NAL
		{0, 0, 0, 3, 0x40, 0x01}, // declared length exceeds payload
	} {
		require.NotPanics(t, func() {
			codec := AVCCToCodec(avcc)
			require.Equal(t, core.CodecH265, codec.Name)
		})
	}
}

func TestTypesStopsAtMalformedNALU(t *testing.T) {
	valid := h264.JoinNALU([]byte{NALUTypePFrame << 1, 1})
	malformed := append(bytes.Clone(valid), 0, 0, 0)

	require.Equal(t, []byte{NALUTypePFrame}, Types(malformed))
	require.Empty(t, Types(nil))
}

func TestSafariPayDropsMalformedAVCC(t *testing.T) {
	for _, payload := range [][]byte{
		nil,
		{0, 0, 0},
		{0, 0, 0, 4, 0x40, 0x01},
	} {
		pay := SafariPay(1200, func(*rtp.Packet) {
			t.Fatal("forwarded malformed AVCC")
		})
		require.NotPanics(t, func() {
			pay(&rtp.Packet{
				Header:  rtp.Header{Version: h264.RTPPacketVersionAVC},
				Payload: payload,
			})
		})
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
