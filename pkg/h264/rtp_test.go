package h264

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// fuA builds an FU-A packet carrying one fragment of a fragmented NAL unit.
func fuA(naluType byte, start, end bool, seq uint16, payload []byte) *rtp.Packet {
	const fuAType = 28
	indicator := byte(0x60) | fuAType // nri=3, type=FU-A
	header := naluType
	if start {
		header |= 0x80
	}
	if end {
		header |= 0x40
	}
	return &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: seq, Marker: end},
		Payload: append([]byte{indicator, header}, payload...),
	}
}

// A depayloader that attaches mid-way through a fragmented IDR must not emit a
// NAL assembled from the leftover fragments. Without a partition-head gate,
// codecs.H264Packet appends the tail fragments and synthesizes a NAL header,
// producing a keyframe-shaped NAL whose slice header is missing - undecodable,
// but indistinguishable from a real keyframe to IsKeyframe.
func TestRTPDepay_SkipsNALUStartedBeforeAttach(t *testing.T) {
	codec := &core.Codec{Name: core.CodecH264}

	var got [][]byte
	depay := RTPDepay(codec, func(packet *core.Packet) {
		got = append(got, append([]byte(nil), packet.Payload...))
	})

	// Attach mid-NAL: the start fragment (and everything before it) was never
	// seen. Only the tail of a fragmented IDR arrives.
	depay(fuA(NALUTypeIFrame, false, false, 100, []byte{0xaa, 0xbb}))
	depay(fuA(NALUTypeIFrame, false, true, 101, []byte{0xcc, 0xdd}))

	require.Empty(t, got, "must not emit a NAL whose start fragment was never received")

	// A complete NAL that begins after we attached must still be delivered.
	depay(fuA(NALUTypeIFrame, true, false, 102, []byte{0x01, 0x02}))
	depay(fuA(NALUTypeIFrame, false, true, 103, []byte{0x03, 0x04}))

	require.Len(t, got, 1, "a NAL received in full must be emitted")
}
