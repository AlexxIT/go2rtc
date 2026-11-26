package h264

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestRepairAVCCDoesNotMutateInputPacket(t *testing.T) {
	codec := &core.Codec{FmtpLine: "sprop-parameter-sets=Zw==,aA=="}
	idr := []byte{NALUTypeIFrame}
	want := JoinNALU([]byte{0x67}, []byte{0x68}, idr)

	for _, test := range []struct {
		name    string
		payload []byte
	}{
		{"IDR", JoinNALU(idr)},
		{"SEI before IDR", JoinNALU([]byte{NALUTypeSEI}, idr)},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := &rtp.Packet{Header: rtp.Header{Timestamp: 123}, Payload: bytes.Clone(test.payload)}
			var output *rtp.Packet
			repair := RepairAVCC(codec, func(packet *rtp.Packet) {
				output = packet
			})
			repair(input)

			require.Equal(t, test.payload, input.Payload)
			require.NotNil(t, output)
			require.NotSame(t, input, output)
			require.Equal(t, input.Header, output.Header)
			require.Equal(t, want, output.Payload)
		})
	}
}

func TestRepairAVCCDropsStandaloneSEI(t *testing.T) {
	input := &rtp.Packet{Payload: JoinNALU([]byte{NALUTypeSEI})}
	repair := RepairAVCC(&core.Codec{}, func(*rtp.Packet) {
		t.Fatal("forwarded a packet without a NAL unit")
	})
	repair(input)
}

func TestRepairAVCCDropsMalformedSEI(t *testing.T) {
	input := &rtp.Packet{Payload: []byte{0, 0, 0, 8, NALUTypeSEI}}
	called := false

	repair := RepairAVCC(&core.Codec{}, func(*rtp.Packet) {
		called = true
	})
	require.NotPanics(t, func() { repair(input) })
	require.False(t, called)
	require.Equal(t, []byte{0, 0, 0, 8, NALUTypeSEI}, input.Payload)
}
