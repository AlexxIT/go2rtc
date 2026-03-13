package rtsp

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestConnWrapPacketHandlerAACRepack(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecAAC,
		ClockRate:   16000,
		PayloadType: 97,
		FmtpLine:    "streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=1408",
	}

	var packets []*rtp.Packet
	conn := &Conn{Repack: true}
	handler := conn.wrapPacketHandler(codec, func(packet *rtp.Packet) {
		clone := *packet
		clone.Payload = append([]byte(nil), packet.Payload...)
		packets = append(packets, &clone)
	})

	handler(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 40000,
			Timestamp:      123456,
			SSRC:           77,
			Marker:         true,
		},
		Payload: []byte{0x00, 0x10, 0x00, 0x40, 1, 2, 3, 4, 5, 6, 7, 8},
	})
	handler(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 17,
			Timestamp:      42,
			SSRC:           88,
			Marker:         true,
		},
		Payload: []byte{0x00, 0x10, 0x00, 0x40, 9, 10, 11, 12, 13, 14, 15, 16},
	})

	require.Len(t, packets, 2)
	require.Equal(t, uint16(0), packets[0].SequenceNumber)
	require.Equal(t, uint16(1), packets[1].SequenceNumber)
	require.Equal(t, uint32(0), packets[0].Timestamp)
	require.Greater(t, packets[1].Timestamp, packets[0].Timestamp)
	require.Zero(t, packets[0].SSRC)
	require.Zero(t, packets[1].SSRC)
}

func TestConnWrapPacketHandlerAACPassthrough(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecAAC,
		ClockRate:   16000,
		PayloadType: 97,
	}

	var packets []*rtp.Packet
	conn := &Conn{}
	handler := conn.wrapPacketHandler(codec, func(packet *rtp.Packet) {
		clone := *packet
		packets = append(packets, &clone)
	})

	handler(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 1234,
			Timestamp:      5678,
			SSRC:           90,
			Marker:         true,
		},
		Payload: []byte{1, 2, 3},
	})

	require.Len(t, packets, 1)
	require.Equal(t, uint16(1234), packets[0].SequenceNumber)
	require.Equal(t, uint32(5678), packets[0].Timestamp)
	require.Equal(t, uint32(90), packets[0].SSRC)
}

func TestConnWrapPacketHandlerH264RepackWaitsForKeyframe(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		ClockRate:   90000,
		PayloadType: 96,
	}

	var packets []*rtp.Packet
	conn := &Conn{Repack: true}
	handler := conn.wrapPacketHandler(codec, func(packet *rtp.Packet) {
		clone := *packet
		clone.Payload = append([]byte(nil), packet.Payload...)
		packets = append(packets, &clone)
	})

	handler(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 10,
			Timestamp:      1000,
			Marker:         true,
		},
		Payload: []byte{0x41, 0x9a},
	})

	require.Len(t, packets, 0)

	handler(&rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			SequenceNumber: 11,
			Timestamp:      4000,
			Marker:         true,
		},
		Payload: []byte{0x65, 0x88},
	})

	require.NotEmpty(t, packets)
}
