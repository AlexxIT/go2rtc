package h265

import (
	"encoding/base64"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func TestDecodeSPS(t *testing.T) {
	s := "QgEBAWAAAAMAAAMAAAMAAAMAmaAAoAgBaH+KrTuiS7/8AAQABbAgApMuADN/mAE="
	b, err := base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.NotNil(t, sps)
	require.Equal(t, uint16(5120), sps.Width())
	require.Equal(t, uint16(1440), sps.Height())
}

func TestDecodeSPS2(t *testing.T) {
	s := "QgEBIUAAAAMAkAAAAwAAAwCWoAUCAWlnpbkShc1AQIC4QAAAAwBAAAAFFEn/eEAOpgAV+V8IBBA="
	b, err := base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.NotNil(t, sps)
	require.Equal(t, uint16(640), sps.Width())
	require.Equal(t, uint16(360), sps.Height())
}

func TestRepairAVCCDropsTruncatedPacket(t *testing.T) {
	codec := &core.Codec{Name: core.CodecH265}
	called := 0
	handler := RepairAVCC(codec, func(*rtp.Packet) {
		called++
	})

	handler(nil)
	for size := 0; size < 5; size++ {
		handler(&rtp.Packet{Payload: make([]byte, size)})
	}
	require.Zero(t, called)

	// Two-byte H.265 IDR NALU with a four-byte AVCC length prefix.
	handler(&rtp.Packet{Payload: []byte{0, 0, 0, 2, 0x26, 0x01}})
	require.Equal(t, 1, called)
}

func TestAVCCToCodecDropsTruncatedPacket(t *testing.T) {
	for _, avcc := range [][]byte{
		nil,
		{0, 0, 0, 0},
		{0, 0, 0, 0, 0x40},
		{0, 0, 0, 1, 0x40},
		{0, 0, 0, 2, 0x40},
	} {
		require.NotPanics(t, func() {
			codec := AVCCToCodec(avcc)
			require.Equal(t, core.CodecH265, codec.Name)
		})
	}
}
