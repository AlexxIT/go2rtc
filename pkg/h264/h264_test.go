package h264

import (
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeConfig(t *testing.T) {
	s := "01640033ffe1000c67640033ac1514a02800f19001000468ee3cb0"
	src, err := hex.DecodeString(s)
	require.Nil(t, err)

	profile, sps, pps := DecodeConfig(src)
	require.NotNil(t, profile)
	require.NotNil(t, sps)
	require.NotNil(t, pps)

	dst := EncodeConfig(sps, pps)
	require.Equal(t, src, dst)
}

func TestDecodeSPS(t *testing.T) {
	s := "Z0IAMukAUAHjQgAAB9IAAOqcCAA=" // Amcrest AD410
	b, err := base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.Equal(t, uint16(2560), sps.Width())
	require.Equal(t, uint16(1920), sps.Height())

	s = "R00AKZmgHgCJ+WEAAAMD6AAATiCE" // Sonoff
	b, err = base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps = DecodeSPS(b)
	require.Equal(t, uint16(1920), sps.Width())
	require.Equal(t, uint16(1080), sps.Height())
}
