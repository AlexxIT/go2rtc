package mpegts

import (
	"encoding/hex"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestTime(t *testing.T) {
	w := NewWriter()
	w.WriteTime(0xFFFFFFFF)
	assert.Equal(t, []byte{0x27, 0xFF, 0xFF, 0xFF, 0xFF}, w.Bytes())
	ts := ParseTime(w.Bytes())
	assert.Equal(t, uint32(0xFFFFFFFF), ts)
}

func dec(s string) []byte {
	s = strings.ReplaceAll(s, " ", "")
	b, _ := hex.DecodeString(s)
	return b
}

func TestStream(t *testing.T) {
	// ffmpeg
	annexb := dec("00000001 09f0 00000001 6764001fac2484014016ec0440000003004000000c23c60c92 00000001 68ee32c8b0 000001 6588808003 00000001 09")
	avc, i := ParseAVC(annexb)
	assert.Equal(t, dec("00000019 6764001fac2484014016ec0440000003004000000c23c60c92 00000005 68ee32c8b0 00000005 6588808003"), avc)
	assert.Equal(t, dec("00000001 09"), annexb[i:])

	// http mpeg ts
	annexb = dec("00000001 0950 000001 6764001facd2014016e8400000fa400030e081 000001 68ea8f2c 000001 65b8400eff 00000001 09")
	avc, i = ParseAVC(annexb)
	assert.Equal(t, dec("00000013 6764001facd2014016e8400000fa400030e081 00000004 68ea8f2c 00000005 65b8400eff"), avc)
	assert.Equal(t, dec("00000001 09"), annexb[i:])

	// tapo TC60
	annexb = dec("00000001 67640028ac1ad00a00b74dc0404050000003001000000301e8f1422a 00000001 68ee04c92240 00000001 45b80000d0 00000001 67")
	avc, i = ParseAVC(annexb)
	assert.Equal(t, dec("0000001C 67640028ac1ad00a00b74dc0404050000003001000000301e8f1422a 00000006 68ee04c92240 00000005 45b80000d0"), avc)
	assert.Equal(t, dec("00000001 67"), annexb[i:])

	// Tapo ?
	annexb = dec("00000001 674d0032e90048014742000007d2000138d108 00000001 68ea8f20 00000001 65b8400cff 00000001 67")
	avc, i = ParseAVC(annexb)
	assert.Equal(t, dec("00000013 674d0032e90048014742000007d2000138d108 00000004 68ea8f20 00000005 65b8400cff"), avc)
	assert.Equal(t, dec("00000001 67"), annexb[i:])
}
