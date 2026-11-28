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

func TestRTPDepayIgnoresFUContinuationWithoutStart(t *testing.T) {
	var output *rtp.Packet
	depay := RTPDepay(&core.Codec{FmtpLine: "sprop-vps=QAE="}, func(packet *rtp.Packet) {
		output = packet
	})

	// End an IDR FU without ending the access unit, leaving the prepended VPS
	// in the buffer. A later orphan continuation must not be treated as active.
	depay(&rtp.Packet{Header: rtp.Header{SequenceNumber: 1}, Payload: []byte{0x62, 1, 0x80 | NALUTypeIFrame, 0xAA}})
	depay(&rtp.Packet{Header: rtp.Header{SequenceNumber: 2}, Payload: []byte{0x62, 1, 0x40 | NALUTypeIFrame, 0xBB}})
	depay(&rtp.Packet{Header: rtp.Header{SequenceNumber: 3, Marker: true}, Payload: []byte{0x62, 1, NALUTypeIFrame, 0xCC}})

	require.Nil(t, output)
}

func TestRTPDepayReassemblesFU(t *testing.T) {
	var output *rtp.Packet
	depay := RTPDepay(&core.Codec{}, func(packet *rtp.Packet) {
		output = packet
	})
	fragments := [][]byte{
		[]byte{0x62, 1, 0x80 | NALUTypeIFrame, 0xAA},
		[]byte{0x62, 1, 0x40 | NALUTypeIFrame, 0xBB, 0xCC},
	}
	for i, payload := range fragments {
		packet := &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(10 + i)},
			Payload: payload,
		}
		packet.Marker = i == len(fragments)-1
		depay(packet)
	}

	require.NotNil(t, output)
	require.Equal(t, h264.JoinNALU([]byte{byte(NALUTypeIFrame << 1), 1, 0xAA, 0xBB, 0xCC}), output.Payload)
}

func TestRTPDepayDropsShortFU(t *testing.T) {
	depay := RTPDepay(&core.Codec{}, func(*rtp.Packet) {
		t.Fatal("forwarded a short FU")
	})
	for _, payload := range [][]byte{{0x62, 1, 0x80}, {0x62, 1, 0x40}} {
		require.NotPanics(t, func() {
			depay(&rtp.Packet{Header: rtp.Header{Marker: true}, Payload: payload})
		})
	}
}

// Haima camera 1920x1080
const testFmtpLine = "profile-id=1;sprop-vps=QAEMAf//IUAAAAMAkAAAAwAAAwCWJQJA;sprop-sps=QgEBIUAAAAMAkAAAAwAAAwCWoAPAgBDlnruRKFzUBAQEEAAAAwAQAAADAUCA;sprop-pps=RAHA88Dm2Q=="

func TestRTPDepayAggregationPacket(t *testing.T) {
	// ffmpeg and go2rtc RTSP output: AP(VPS, SPS, PPS), FU(IDR)...
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 300)
	pFrame := testNALU(NALUTypePFrame, 100)
	sei := testNALU(NALUTypePrefixSEI, 30)

	fu := testFU(idr, 120)
	require.Len(t, fu, 3)

	codec := &core.Codec{
		Name:      core.CodecH265,
		ClockRate: 90000,
		FmtpLine:  testFmtpLine,
	}

	aus := testDepay(codec,
		testAU(testAP(vps, sps, pps), fu[0], fu[1], fu[2]),
		testAU(testAP(pFrame, sei)),
	)

	require.Len(t, aus, 2)

	// Parameter sets from the AP, without a second copy from the SDP.
	require.Equal(t, h264.JoinNALU(vps, sps, pps, idr), aus[0])
	require.True(t, IsKeyframe(aus[0]))
	require.Equal(t, h264.JoinNALU(pFrame, sei), aus[1])
}

func TestRTPDepayAggregationPacketBroken(t *testing.T) {
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 100)

	ap := testAP(vps, sps, pps)

	for _, tt := range []struct {
		name    string
		payload []byte
	}{
		{"empty", []byte{h265NaluAggregationPacketType << 1, 1}},
		{"short length", []byte{h265NaluAggregationPacketType << 1, 1, 0}},
		{"zero length", []byte{h265NaluAggregationPacketType << 1, 1, 0, 0}},
		{"short NALU", []byte{h265NaluAggregationPacketType << 1, 1, 0, 1, 0}},
		{"truncated NALU", ap[:len(ap)-1]},
		{"trailing byte", append(bytes.Clone(ap), 0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			aus := testDepay(&core.Codec{},
				testAU(vps, tt.payload, idr),
				testAU(testAP(vps, sps, pps), idr),
			)

			require.Len(t, aus, 2)

			// No buffered NALUs from before or inside the broken AP.
			require.Equal(t, h264.JoinNALU(idr), aus[0])

			// The next access unit is unaffected.
			require.Equal(t, h264.JoinNALU(vps, sps, pps, idr), aus[1])
		})
	}
}

func TestRTPDepayMarkedSEI(t *testing.T) {
	// Haima cameras end access units with small marked SEI packets.
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 300)
	pFrame1 := testNALU(NALUTypePFrame, 300)
	pFrame2 := testNALU(NALUTypePFrame, 100)
	prefixSEI := testNALU(NALUTypePrefixSEI, 70)
	suffixSEI := testNALU(NALUTypeSuffixSEI, 20)

	fuI := testFU(idr, 120)
	fuP := testFU(pFrame1, 120)

	aus := testDepay(&core.Codec{},
		testAU(vps, sps, pps, fuI[0], fuI[1], fuI[2], prefixSEI),
		testAU(fuP[0], fuP[1], fuP[2], prefixSEI),
		testAU(pFrame2, suffixSEI),
		testAU(testAP(vps, sps, pps), fuI[0], fuI[1], fuI[2], prefixSEI),
	)

	require.Len(t, aus, 4)
	require.Equal(t, h264.JoinNALU(vps, sps, pps, idr, prefixSEI), aus[0])
	require.True(t, IsKeyframe(aus[0]))
	require.Equal(t, h264.JoinNALU(pFrame1, prefixSEI), aus[1])
	require.Equal(t, h264.JoinNALU(pFrame2, suffixSEI), aus[2])
	require.Equal(t, h264.JoinNALU(vps, sps, pps, idr, prefixSEI), aus[3])
}

func TestRTPDepayMarkedNALUnits(t *testing.T) {
	// RtspServer https://github.com/AlexxIT/go2rtc/issues/244
	// sends marked VPS, marked SPS, marked PPS, marked SEI, marked IFrame.
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 300)
	sei := testNALU(NALUTypePrefixSEI, 30)

	fu := testFU(idr, 120)

	aus := testDepay(&core.Codec{},
		testAU(vps),
		testAU(sps),
		testAU(pps),
		testAU(sei),
		testAU(fu[0], fu[1], fu[2]),
	)

	require.Len(t, aus, 1)

	// SEI before picture data is skipped without breaking the sequence.
	require.Equal(t, h264.JoinNALU(vps, sps, pps, idr), aus[0])
}

func TestRTPDepayInterruptedFU(t *testing.T) {
	idr := testNALU(NALUTypeIFrame, 300)
	pFrame := testNALU(NALUTypePFrame, 100)
	sei := testNALU(NALUTypePrefixSEI, 30)
	fu := testFU(idr, 120)

	for _, tt := range []struct {
		name    string
		payload []byte
	}{
		{"AP", testAP(pFrame, sei)},
		{"SEI", sei},
		{"VPS", testNALU(NALUTypeVPS, 20)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			aus := testDepay(&core.Codec{},
				testAU(fu[0], tt.payload),
				testAU(fu[1], fu[2]),
				testAU(pFrame),
			)

			// A non-FU packet interrupts the unfinished NALU.
			// Its remaining FU fragments must also be discarded.
			require.Len(t, aus, 1)
			require.Equal(t, h264.JoinNALU(pFrame), aus[0])
		})
	}
}

func TestRTPDepayMarkedSEIAfterPacketLoss(t *testing.T) {
	idr := testNALU(NALUTypeIFrame, 100)
	sei := testNALU(NALUTypePrefixSEI, 30)
	pFrame := testNALU(NALUTypePFrame, 100)

	var aus [][]byte

	depay := RTPDepay(&core.Codec{}, func(packet *rtp.Packet) {
		aus = append(aus, bytes.Clone(packet.Payload))
	})

	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 1},
		Payload: idr,
	})

	// Packet 2 is lost. The SEI must not complete a broken access unit.
	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 3, Marker: true},
		Payload: sei,
	})

	require.Empty(t, aus)

	// Recover on the next access unit.
	depay(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 4, Marker: true},
		Payload: pFrame,
	})

	require.Len(t, aus, 1)
	require.Equal(t, h264.JoinNALU(pFrame), aus[0])
}

func TestRTPPayDepayAggregation(t *testing.T) {
	vps, sps, pps := GetParameterSet(testFmtpLine)
	idr := testNALU(NALUTypeIFrame, 100)
	sei := testNALU(NALUTypePrefixSEI, 30)

	payload := h264.JoinNALU(vps, sps, pps, idr, sei)

	var aus [][]byte
	var packets int

	depay := RTPDepay(&core.Codec{}, func(packet *rtp.Packet) {
		aus = append(aus, bytes.Clone(packet.Payload))
	})

	pay := RTPPay(1472, func(packet *rtp.Packet) {
		packets++

		// All NALUs fit into one aggregation packet.
		require.Equal(t, byte(h265NaluAggregationPacketType), (packet.Payload[0]>>1)&0x3F)

		depay(packet)
	})

	pay(&rtp.Packet{
		Header: rtp.Header{
			Version: h264.RTPPacketVersionAVC,
		},
		Payload: payload,
	})

	require.Equal(t, 1, packets)
	require.Len(t, aus, 1)
	require.Equal(t, payload, aus[0])
}

// testNALU returns a NAL unit with header and payload of the given size.
func testNALU(nuType byte, size int) []byte {
	b := make([]byte, 2+size)
	b[0] = nuType << 1
	b[1] = 1 // TID

	for i := 2; i < len(b); i++ {
		b[i] = byte(i)
	}

	return b
}

// testAP joins NAL units into one Aggregation Packet (RFC 7798 4.4.2).
func testAP(nalus ...[]byte) []byte {
	b := []byte{h265NaluAggregationPacketType << 1, 1}

	for _, nalu := range nalus {
		b = binary.BigEndian.AppendUint16(b, uint16(len(nalu)))
		b = append(b, nalu...)
	}

	return b
}

// testFU splits a NAL unit into Fragmentation Units (RFC 7798 4.4.3).
func testFU(nalu []byte, size int) (payloads [][]byte) {
	nuType := (nalu[0] >> 1) & 0x3F

	for b := nalu[2:]; len(b) > 0; {
		n := min(size, len(b))
		header := nuType

		if len(payloads) == 0 {
			header |= 0x80 // start
		}
		if n == len(b) {
			header |= 0x40 // end
		}

		payload := []byte{nalu[0]&0x81 | NALUTypeFU<<1, nalu[1], header}
		payloads = append(payloads, append(payload, b[:n]...))
		b = b[n:]
	}

	return
}

// testAU returns RTP packets with the marker on the last packet.
func testAU(payloads ...[]byte) []*rtp.Packet {
	packets := make([]*rtp.Packet, len(payloads))

	for i, payload := range payloads {
		packets[i] = &rtp.Packet{Payload: payload}
	}

	packets[len(packets)-1].Marker = true

	return packets
}

// testDepay returns AVCC access units from consecutive RTP packets.
func testDepay(codec *core.Codec, aus ...[]*rtp.Packet) (payloads [][]byte) {
	depay := RTPDepay(codec, func(packet *rtp.Packet) {
		// RTPDepay reuses its buffer for the next access unit.
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
