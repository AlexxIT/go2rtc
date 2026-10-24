package h264

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// fu builds an FU-A fragment of a NAL of type nalType (RFC 6184 5.8).
func fu(nalType byte, start, end, marker bool, ts uint32, body ...byte) *rtp.Packet {
	hdr := nalType
	if start {
		hdr |= 0x80
	}
	if end {
		hdr |= 0x40
	}
	return &rtp.Packet{
		Header:  rtp.Header{Marker: marker, Timestamp: ts},
		Payload: append([]byte{0x60 | fuaNALUType, hdr}, body...),
	}
}

func depayAll(packets ...*rtp.Packet) (types []byte) {
	h := RTPDepay(&core.Codec{Name: core.CodecH264, ClockRate: 90000}, func(p *core.Packet) {
		types = append(types, NALUType(p.Payload))
	})
	for _, p := range packets {
		h(p)
	}
	return
}

// A consumer that joins in the middle of a fragmented IDR must not turn its
// tail into a "keyframe": that is a headless IDR no decoder can use.
func TestRTPDepayDropsFragmentsWithoutStart(t *testing.T) {
	types := depayAll(
		fu(NALUTypeIFrame, false, false, false, 1, 0xaa), // joined mid-IDR
		fu(NALUTypeIFrame, false, true, true, 1, 0xbb),
		fu(NALUTypePFrame, true, false, false, 2, 0x9a), // the next frame is whole
		fu(NALUTypePFrame, false, true, true, 2, 0xcc),
	)
	require.Equal(t, []byte{NALUTypePFrame}, types)
}

// A start fragment while an earlier NAL is unfinished (its end was lost)
// begins a new NAL instead of being appended to the stale one.
func TestRTPDepayRestartsAtStartFragment(t *testing.T) {
	var got []byte
	h := RTPDepay(&core.Codec{Name: core.CodecH264, ClockRate: 90000}, func(p *core.Packet) {
		got = append([]byte{}, p.Payload...)
	})
	h(fu(NALUTypePFrame, true, false, false, 1, 0x11, 0x11)) // end lost
	h(fu(NALUTypePFrame, true, false, false, 2, 0x22))
	h(fu(NALUTypePFrame, false, true, true, 2, 0x33))
	require.Equal(t, []byte{0, 0, 0, 3, 0x61, 0x22, 0x33}, got)
}
