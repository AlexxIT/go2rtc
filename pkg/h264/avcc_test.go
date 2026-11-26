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
		{"empty NAL before IDR", append(make([]byte, 4), JoinNALU(idr)...)},
		{"empty NAL between SEI and IDR", append(append(JoinNALU([]byte{NALUTypeSEI}), 0, 0, 0, 0), JoinNALU(idr)...)},
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

func TestRepairAVCCDropsMalformedPayload(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload []byte
	}{
		{"empty payload", nil},
		{"empty NAL", make([]byte, 4)},
		{"truncated SEI", []byte{0, 0, 0, 8, NALUTypeSEI}},
		{"oversized NAL", []byte{0xFF, 0xFF, 0xFF, 0xFF, NALUTypeSEI}},
		{"SEI with short tail", append(JoinNALU([]byte{NALUTypeSEI}), 0, 0, 0)},
		{"IDR with short tail", append(JoinNALU([]byte{NALUTypeIFrame}), 0, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repair := RepairAVCC(&core.Codec{}, func(*rtp.Packet) {
				t.Fatal("forwarded a malformed AVCC packet")
			})
			original := bytes.Clone(test.payload)
			repair(&rtp.Packet{Payload: test.payload})
			require.Equal(t, original, test.payload)
		})
	}
}

func TestNALUTypeShortPayload(t *testing.T) {
	for size := range 5 {
		require.Equal(t, byte(0xFF), NALUType(make([]byte, size)))
	}
}
