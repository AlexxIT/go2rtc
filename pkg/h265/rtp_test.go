package h265

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// Haima camera 1920x1080
const testFmtpLine = "profile-id=1;sprop-vps=QAEMAf//IUAAAAMAkAAAAwAAAwCWJQJA;sprop-sps=QgEBIUAAAAMAkAAAAwAAAwCWoAPAgBDlnruRKFzUBAQEEAAAAwAQAAADAUCA;sprop-pps=RAHA88Dm2Q=="

func TestRTPDepayAggregationPacket(t *testing.T) {
	// ffmpeg and go2rtc RTSP output: AP(VPS, SPS, PPS), FU(IDR)...
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 300)
	pFrame := testNALU(NALUTypePFrame, 100)
	sei := testNALU(NALUTypePrefixSEI, 30)

	fu := testFU(idr, 120) // start, continue, end
	require.Len(t, fu, 3)

	codec := &core.Codec{Name: core.CodecH265, ClockRate: 90000, FmtpLine: testFmtpLine}
	aus := testDepay(codec,
		testAU(testAP(vps, sps, pps), fu[0], fu[1], fu[2]),
		testAU(testAP(pFrame, sei)),
	)
	require.Len(t, aus, 2)

	// parameter sets from the AP, without a second copy from the SDP
	require.Equal(t, []byte{NALUTypeVPS, NALUTypeSPS, NALUTypePPS, NALUTypeIFrame}, Types(aus[0]))
	require.Equal(t, h264.JoinNALU(vps, sps, pps, idr), aus[0])
	require.True(t, IsKeyframe(aus[0]))

	require.Equal(t, []byte{NALUTypePFrame, NALUTypePrefixSEI}, Types(aus[1]))
	require.Equal(t, h264.JoinNALU(pFrame, sei), aus[1])
}

func TestRTPDepayAggregationPacketBroken(t *testing.T) {
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 100)

	ap := testAP(vps, sps, pps)
	ap = ap[:len(ap)-1] // last NAL unit is shorter than its size

	aus := testDepay(&core.Codec{Name: core.CodecH265},
		testAU(ap, idr),
		testAU(testAP(vps, sps, pps), idr),
	)
	require.Len(t, aus, 2)

	// no NAL units from the broken AP
	require.Equal(t, []byte{NALUTypeIFrame}, Types(aus[0]))
	require.Equal(t, h264.JoinNALU(idr), aus[0])
	require.Equal(t, h264.JoinNALU(vps, sps, pps, idr), aus[1])
}

// testNALU returns NAL unit with header and payload of the given size
func testNALU(nuType byte, size int) []byte {
	b := make([]byte, 2+size)
	b[0] = nuType << 1
	b[1] = 1 // TID
	for i := 2; i < len(b); i++ {
		b[i] = byte(i)
	}
	return b
}

// testAP joins NAL units into one Aggregation Packet (RFC 7798 4.4.2)
func testAP(nalus ...[]byte) []byte {
	b := []byte{NALUTypeAP << 1, 1}
	for _, nalu := range nalus {
		b = binary.BigEndian.AppendUint16(b, uint16(len(nalu)))
		b = append(b, nalu...)
	}
	return b
}

// testFU splits NAL unit into Fragmentation Units (RFC 7798 4.4.3)
func testFU(nalu []byte, size int) (payloads [][]byte) {
	nuType := (nalu[0] >> 1) & 0x3F
	for b := nalu[2:]; len(b) > 0; {
		n := min(size, len(b))

		header := nuType
		if len(payloads) == 0 {
			header |= 0b10000000 // start
		}
		if n == len(b) {
			header |= 0b01000000 // end
		}

		payload := []byte{nalu[0]&0x81 | NALUTypeFU<<1, nalu[1], header}
		payloads = append(payloads, append(payload, b[:n]...))
		b = b[n:]
	}
	return
}

// testAU returns RTP packets of one Access Unit, with the marker on the last packet
func testAU(payloads ...[]byte) []*rtp.Packet {
	packets := make([]*rtp.Packet, len(payloads))
	for i, payload := range payloads {
		packets[i] = &rtp.Packet{Payload: payload}
	}
	packets[len(packets)-1].Marker = true
	return packets
}

// testDepay passes packets with consecutive sequence numbers through RTPDepay
// and returns the AVCC payload of each output Access Unit
func testDepay(codec *core.Codec, aus ...[]*rtp.Packet) (payloads [][]byte) {
	depay := RTPDepay(codec, func(packet *rtp.Packet) {
		// RTPDepay reuses its buffer for the next Access Unit
		payloads = append(payloads, bytes.Clone(packet.Payload))
	})

	var seq uint16 = 65530 // check rollover
	for _, au := range aus {
		for _, packet := range au {
			packet.Version = 2
			packet.SequenceNumber = seq
			seq++
			depay(packet)
		}
	}
	return
}
