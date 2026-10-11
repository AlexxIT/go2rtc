package h264

import (
	"encoding/hex"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRepairAVCCDropsMalformedPayload(t *testing.T) {
	for _, payload := range [][]byte{
		nil, {0}, {0, 0}, {0, 0, 0}, {0, 0, 0, 1}, {0, 0, 0, 0, 1},
		{0, 0, 0, 2, 1}, {0xff, 0xff, 0xff, 0xff, 6},
		JoinNALU([]byte{6}),
		append(JoinNALU([]byte{6}), 0, 0, 0, 1),
		append(JoinNALU([]byte{1}), 0, 0, 0, 2, 1),
		append(JoinNALU([]byte{5}), 0, 0, 0, 0),
		append([]byte{0, 0, 0, 0}, JoinNALU([]byte{1, 2})...),
		append(append(JoinNALU([]byte{1, 2}), 0, 0, 0, 0), JoinNALU([]byte{1, 3})...),
	} {
		t.Run(hex.EncodeToString(payload), func(t *testing.T) {
			called := false
			handler := RepairAVCC(&core.Codec{}, func(*rtp.Packet) { called = true })
			require.NotPanics(t, func() { handler(&rtp.Packet{Payload: payload}) })
			require.False(t, called)
		})
	}
}

func TestRepairAVCCValidPayload(t *testing.T) {
	codec := &core.Codec{FmtpLine: "sprop-parameter-sets=Zw==,aA=="}
	for _, test := range []struct {
		name            string
		input, expected []byte
	}{
		{"P frame", JoinNALU([]byte{1, 2}), JoinNALU([]byte{1, 2})},
		{"I frame", JoinNALU([]byte{5, 2}), JoinNALU([]byte{0x67}, []byte{0x68}, []byte{5, 2})},
		{"SEI and I frame", JoinNALU([]byte{6, 2}, []byte{5, 3}), JoinNALU([]byte{0x67}, []byte{0x68}, []byte{5, 3})},
		{"multiple NALUs", JoinNALU([]byte{1, 2}, []byte{1, 3}), JoinNALU([]byte{1, 2}, []byte{1, 3})},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := &rtp.Packet{Header: rtp.Header{Timestamp: 1234, SequenceNumber: 12}, Payload: test.input}
			called := false
			RepairAVCC(codec, func(actual *rtp.Packet) {
				called = true
				require.Same(t, packet, actual)
				require.Equal(t, test.expected, actual.Payload)
				require.Equal(t, uint32(1234), actual.Timestamp)
				require.Equal(t, uint16(12), actual.SequenceNumber)
			})(packet)
			require.True(t, called)
		})
	}
}
