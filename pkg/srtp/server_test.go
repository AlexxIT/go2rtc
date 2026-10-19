package srtp

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPacketKindAndSSRC_RTPDynamicPayloadTypes(t *testing.T) {
	for _, payloadType := range []byte{0, 8, 96, 99, 110, 111, 0x80 | 111} {
		packet := make([]byte, 12)
		packet[0] = 0x80
		packet[1] = payloadType
		binary.BigEndian.PutUint32(packet[8:], 0x12345678)

		kind, ssrc := packetKindAndSSRC(packet)

		require.Equal(t, packetKindRTP, kind, "payload type %d", payloadType)
		require.Equal(t, uint32(0x12345678), ssrc)
	}
}

func TestPacketKindAndSSRC_RTCP(t *testing.T) {
	packet := make([]byte, 8)
	packet[0] = 0x80
	packet[1] = 200
	binary.BigEndian.PutUint32(packet[4:], 0x87654321)

	kind, ssrc := packetKindAndSSRC(packet)

	require.Equal(t, packetKindRTCP, kind)
	require.Equal(t, uint32(0x87654321), ssrc)
}

func TestPacketKindAndSSRC_IgnoresNonMedia(t *testing.T) {
	for _, payloadType := range []byte{13, 64, 95} {
		packet := make([]byte, 12)
		packet[0] = 0x80
		packet[1] = payloadType
		binary.BigEndian.PutUint32(packet[8:], 0x12345678)

		kind, ssrc := packetKindAndSSRC(packet)

		require.Equal(t, packetKindUnknown, kind, "payload type %d", payloadType)
		require.Zero(t, ssrc)
	}
}
