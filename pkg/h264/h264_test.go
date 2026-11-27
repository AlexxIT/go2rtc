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

func TestGetParameterSetStartCode(t *testing.T) {
	// HiSilicon camera with Annex B start codes inside sprop-parameter-sets
	// https://github.com/AlexxIT/go2rtc/issues/2729
	s := "packetization-mode=1;sprop-parameter-sets=AAAAASdNADPnQGQc01AQEB8AAAMAAQAAAwAZNoAlgOF//wKA,AAAAASjuPIA="

	sps, pps := GetParameterSet(s)
	require.Equal(t, byte(NALUTypeSPS), sps[0]&0x1F)
	require.Equal(t, "28ee3c80", hex.EncodeToString(pps))

	info := DecodeSPS(sps)
	require.Equal(t, uint16(800), info.Width())
	require.Equal(t, uint16(448), info.Height())

	require.Equal(t, "4D0029", GetProfileLevelID(s))
}

func TestDecodeSPS2(t *testing.T) {
	s := "6764001fad84010c20086100430802184010c200843b50740932"
	b, err := hex.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.Equal(t, uint16(928), sps.Width())
	require.Equal(t, uint16(576), sps.Height())

	s = "Z2QAHq2EAQwgCGEAQwgCGEAQwgCEO1BQF/yzcBAQFAAAD6AAAXcCEA==" // unknown
	b, err = base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps = DecodeSPS(b)
	require.Equal(t, uint16(640), sps.Width())
	require.Equal(t, uint16(360), sps.Height())
}

func TestGetFmtpLine(t *testing.T) {
	sps := []byte{0x67, 0x64, 0, 0x1f, 0xac, 0x24}
	pps := []byte{0x68, 0xee, 0x32, 0xc8, 0xb0}
	const mode = "packetization-mode=1"
	const withSPS = mode + ";profile-level-id=64001f;sprop-parameter-sets=Z2QAH6wk"
	const withPair = withSPS + ",aO4yyLA="

	for _, test := range []struct {
		name string
		avc  []byte
		want string
	}{
		{"empty", nil, mode},
		{"short length prefix", []byte{0, 0, 0}, mode},
		{"zero length", []byte{0, 0, 0, 0, NALUTypeSPS}, mode},
		{"oversized length", []byte{0xff, 0xff, 0xff, 0xff, NALUTypeSPS}, mode},
		{"short SPS", JoinNALU(sps[:3]), mode},
		{"short SPS followed by valid pair", JoinNALU(sps[:3], sps, pps), withPair},
		{"SPS followed by truncated PPS", append(JoinNALU(sps), 0, 0, 0, 5, NALUTypePPS), withSPS},
		{"SPS and PPS followed by IDR", JoinNALU(sps, pps, []byte{NALUTypeIFrame}), withPair},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, GetFmtpLine(test.avc))
		})
	}
}

func TestAVCCToCodec(t *testing.T) {
	s := "000000196764001fac2484014016ec0440000003004000000c23c60c920000000568ee32c8b0000000d365"
	b, _ := hex.DecodeString(s)
	codec := AVCCToCodec(b)
	require.Equal(t, "packetization-mode=1;profile-level-id=64001f;sprop-parameter-sets=Z2QAH6wkhAFAFuwEQAAAAwBAAAAMI8YMkg==,aO4yyLA=", codec.FmtpLine)
}
