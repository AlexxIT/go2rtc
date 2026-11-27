package h265

import (
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
