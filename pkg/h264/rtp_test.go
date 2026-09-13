package h264

import (
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

// fragmentedIDR returns one big AVCC IDR NAL and its FU-A RTP packets.
func fragmentedIDR(t *testing.T) (avcc []byte, packets []*rtp.Packet) {
	t.Helper()

	nalu := make([]byte, 3000)
	nalu[0] = 0x65 // IDR, nal_ref_idc=3
	for i := 1; i < len(nalu); i++ {
		nalu[i] = byte(i)
	}
	avcc = make([]byte, 4+len(nalu))
	binary.BigEndian.PutUint32(avcc, uint32(len(nalu)))
	copy(avcc[4:], nalu)

	payloader := &Payloader{IsAVC: true}
	payloads := payloader.Payload(200, avcc)
	require.Greater(t, len(payloads), 3, "expected the NAL to be fragmented")

	for i, p := range payloads {
		packets = append(packets, &rtp.Packet{
			Header:  rtp.Header{Version: 2, Marker: i == len(payloads)-1, SequenceNumber: uint16(i)},
			Payload: p,
		})
	}
	return
}

func depay(t *testing.T) (core.HandlerFunc, *[][]byte) {
	t.Helper()
	var got [][]byte
	handler := RTPDepay(&core.Codec{Name: core.CodecH264}, func(packet *rtp.Packet) {
		got = append(got, packet.Payload)
	})
	return handler, &got
}

// A consumer that attaches while an FU-A fragmented NAL is in flight has no
// slice header for it; the tail must be dropped, not passed on as a bogus NAL.
func TestRTPDepayDropsFUATailWithoutStart(t *testing.T) {
	avcc, packets := fragmentedIDR(t)
	handler, got := depay(t)

	// join mid-NAL: skip the fragment carrying the start bit
	for _, p := range packets[2:] {
		handler(p)
	}
	require.Empty(t, *got, "tail fragments without a start bit produced a NAL")

	// the next complete NAL must still come through intact
	for _, p := range packets {
		handler(p)
	}
	require.Len(t, *got, 1)
	require.Equal(t, avcc, (*got)[0])
}

// A start fragment arriving while a previous fragment set never finished
// (lost end bit) must discard the stale partial data.
func TestRTPDepayRestartsOnNewFUAStart(t *testing.T) {
	avcc, packets := fragmentedIDR(t)
	handler, got := depay(t)

	// first NAL loses its tail
	for _, p := range packets[:len(packets)-2] {
		handler(p)
	}
	// second NAL arrives complete
	for _, p := range packets {
		handler(p)
	}
	require.Len(t, *got, 1)
	require.Equal(t, avcc, (*got)[0])
}

// renumber gives packets consecutive sequence numbers starting at first.
func renumber(packets []*rtp.Packet, first uint16) []*rtp.Packet {
	out := make([]*rtp.Packet, len(packets))
	for i, p := range packets {
		c := *p
		c.SequenceNumber = first + uint16(i)
		out[i] = &c
	}
	return out
}

// pFrame is a single-NAL P-frame packet that completes an access unit.
func pFrame(seq uint16) *rtp.Packet {
	return &rtp.Packet{
		Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: seq},
		Payload: []byte{0x41, 0x9a, 0x00, 0x11, 0x22},
	}
}

// A fragment lost from the middle of an IDR must not produce a keyframe with
// a hole in it; the next complete IDR is passed on.
func TestRTPDepayDropsIDRWithLostFragment(t *testing.T) {
	avcc, packets := fragmentedIDR(t)
	handler, got := depay(t)

	damaged := renumber(packets, 100)
	for i, p := range damaged {
		if i == 2 {
			continue // lost in transit
		}
		handler(p)
	}
	require.Empty(t, *got, "an IDR missing a fragment was passed on")

	for _, p := range renumber(packets, 100+uint16(len(packets))) {
		handler(p)
	}
	require.Len(t, *got, 1)
	require.Equal(t, avcc, (*got)[0])
}

// After any loss, dependent pictures are dropped until a keyframe arrives.
func TestRTPDepayWaitsForKeyframeAfterLoss(t *testing.T) {
	avcc, packets := fragmentedIDR(t)
	handler, got := depay(t)

	n := uint16(len(packets))
	for _, p := range renumber(packets, 0) {
		handler(p)
	}
	handler(pFrame(n))
	require.Len(t, *got, 2, "clean IDR and P-frame should pass")

	handler(pFrame(n + 2)) // n+1 was lost
	handler(pFrame(n + 3))
	require.Len(t, *got, 2, "P-frames after a loss must be dropped")

	for _, p := range renumber(packets, n+4) {
		handler(p)
	}
	handler(pFrame(n + 4 + n))
	require.Len(t, *got, 4, "recovery keyframe and the P-frame after it should pass")
	require.Equal(t, avcc, (*got)[2])
}

// A sequence number wrapping from 65535 to 0 is continuity, not loss.
func TestRTPDepaySequenceRollover(t *testing.T) {
	avcc, packets := fragmentedIDR(t)
	handler, got := depay(t)

	for _, p := range renumber(packets, 65535-uint16(len(packets)/2)) {
		handler(p)
	}
	require.Len(t, *got, 1)
	require.Equal(t, avcc, (*got)[0])
}
