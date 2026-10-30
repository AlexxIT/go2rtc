package h265

import (
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRTPDepayAggregationPacket(t *testing.T) {
	payload := []byte{
		0x60, 0x01, // AP header (NAL type 48)
		0x00, 0x03, 0x40, 0x01, 0xaa, // VPS NAL
		0x00, 0x03, 0x42, 0x01, 0xbb, // SPS NAL
	}
	var got *rtp.Packet
	depay := RTPDepay(&core.Codec{}, func(packet *rtp.Packet) { got = packet })
	depay(&rtp.Packet{
		Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: 1},
		Payload: payload,
	})

	require.NotNil(t, got)
	require.Equal(t, []byte{
		0, 0, 0, 3, 0x40, 0x01, 0xaa,
		0, 0, 0, 3, 0x42, 0x01, 0xbb,
	}, got.Payload)
	require.Equal(t, byte(2), got.Version)
	require.True(t, got.Marker)
}

func TestRTPDepayAggregationPacketWithDON(t *testing.T) {
	payload := []byte{
		0x60, 0x01, // AP header
		0x00, 0x05, // DONL for first unit
		0x00, 0x03, 0x40, 0x01, 0xaa, // first NAL
		0x00, // DOND for second unit
		0x00, 0x03, 0x42, 0x01, 0xbb, // second NAL
	}
	var got *rtp.Packet
	depay := RTPDepay(&core.Codec{FmtpLine: "sprop-max-don-diff=1"}, func(packet *rtp.Packet) { got = packet })
	depay(&rtp.Packet{
		Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: 1},
		Payload: payload,
	})

	require.NotNil(t, got)
	require.Equal(t, []byte{
		0, 0, 0, 3, 0x40, 0x01, 0xaa,
		0, 0, 0, 3, 0x42, 0x01, 0xbb,
	}, got.Payload)
}

func TestRTPDepayMalformedAggregationPacketIsDropped(t *testing.T) {
	payload := []byte{0x60, 0x01, 0x00, 0x05, 0x40, 0x01}
	called := false
	depay := RTPDepay(&core.Codec{}, func(*rtp.Packet) { called = true })
	depay(&rtp.Packet{
		Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: 1},
		Payload: payload,
	})

	require.False(t, called)
	_ = binary.BigEndian // keep the import available for packet fixture length checks
}
