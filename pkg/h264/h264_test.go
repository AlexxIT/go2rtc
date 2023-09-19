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

	s = "Z01AMqaAKAC1kAA=" // Dahua
	b, err = base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps = DecodeSPS(b)
	require.Equal(t, uint16(2560), sps.Width())
	require.Equal(t, uint16(1440), sps.Height())

	s = "Z2QAM6wVFKAoAPGQ" // Reolink
	b, err = base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps = DecodeSPS(b)
	require.Equal(t, uint16(2560), sps.Width())
	require.Equal(t, uint16(1920), sps.Height())

	s = "Z2QAKKwa0AoAt03AQEBQAAADABAAAAMB6PFCKg==" // TP-Link
	b, err = base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps = DecodeSPS(b)
	require.Equal(t, uint16(1280), sps.Width())
	require.Equal(t, uint16(720), sps.Height())

	s = "Z2QAFqwa0BQF/yzcBAQFAAADAAEAAAMAHo8UIqA=" // TP-Link sub
	b, err = base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps = DecodeSPS(b)
	require.Equal(t, uint16(640), sps.Width())
	require.Equal(t, uint16(360), sps.Height())
}

func TestGetProfileLevelID(t *testing.T) {
	// OpenIPC https://github.com/OpenIPC
	s := "profile-level-id=0033e7; packetization-mode=1; "
	profile := GetProfileLevelID(s)
	require.Equal(t, "640029", profile)

	// Eufy T8400 https://github.com/AlexxIT/go2rtc/issues/155
	s = "packetization-mode=1;profile-level-id=276400"
	profile = GetProfileLevelID(s)
	require.Equal(t, "640029", profile)
}
