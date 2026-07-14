package h265

import (
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// buildAP wraps NAL units into an RFC 7798 §4.4.2 Aggregation Packet:
// [PayloadHdr (type=48)] [2-byte size][NALU]*  (no DONL — sprop-max-don-diff=0,
// which is what libavformat's HEVC RTP packetizer emits).
func buildAP(nalus ...[]byte) []byte {
	ap := []byte{NALUTypeAP << 1, 0x01}
	for _, nalu := range nalus {
		ap = binary.BigEndian.AppendUint16(ap, uint16(len(nalu)))
		ap = append(ap, nalu...)
	}
	return ap
}

// TestRTPDepayAggregationPacket reproduces the production failure behind
// black thumbnails / "exit status 183" for HEVC cameras behind exec:ffmpeg
// sources: libavformat bundles VPS+SPS+PPS into one type-48 Aggregation
// Packet, which the depacketizer must split into individual AVCC
// length-prefixed NAL units. Without AP support the whole packet is emitted
// as a single bogus NAL of type 48 — an unspecified non-VCL type that breaks
// every downstream decoder.
func TestRTPDepayAggregationPacket(t *testing.T) {
	vps := []byte{NALUTypeVPS << 1, 0x01, 0xAA, 0xBB, 0xCC}
	sps := []byte{NALUTypeSPS << 1, 0x01, 0x11, 0x22, 0x33, 0x44}
	pps := []byte{NALUTypePPS << 1, 0x01, 0x55}
	idr := []byte{NALUTypeIFrame << 1, 0x01, 0xDE, 0xAD, 0xBE, 0xEF}

	var got []*rtp.Packet
	depay := RTPDepay(&core.Codec{}, func(packet *rtp.Packet) {
		got = append(got, packet)
	})

	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 100, Marker: false},
		Payload: buildAP(vps, sps, pps),
	})
	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 101, Marker: true},
		Payload: idr,
	})

	require.Len(t, got, 1)
	require.Equal(t, []byte{NALUTypeVPS, NALUTypeSPS, NALUTypePPS, NALUTypeIFrame}, Types(got[0].Payload))

	// Each NAL must round-trip byte-identical with its own AVCC length prefix.
	payload := got[0].Payload
	for _, want := range [][]byte{vps, sps, pps, idr} {
		size := int(binary.BigEndian.Uint32(payload))
		require.Equal(t, len(want), size)
		require.Equal(t, want, payload[4:4+size])
		payload = payload[4+size:]
	}
	require.Empty(t, payload)
}

// TestRTPDepayAggregationPacketMalformed verifies the corruption-drop
// convention (same as the FU branch): a truncated AP must clear the buffer
// and emit nothing, and the next intact access unit must still go through.
func TestRTPDepayAggregationPacketMalformed(t *testing.T) {
	var got []*rtp.Packet
	depay := RTPDepay(&core.Codec{}, func(packet *rtp.Packet) {
		got = append(got, packet)
	})

	// Size field claims 200 bytes but only 3 follow.
	malformed := []byte{NALUTypeAP << 1, 0x01, 0x00, 200, 0x40, 0x01, 0xFF}
	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 50, Marker: false},
		Payload: malformed,
	})
	require.Empty(t, got)

	idr := []byte{NALUTypeIFrame << 1, 0x01, 0xDE, 0xAD}
	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 51, Marker: true},
		Payload: idr,
	})

	require.Len(t, got, 1)
	require.Equal(t, []byte{NALUTypeIFrame}, Types(got[0].Payload))
}
