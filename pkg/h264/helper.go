package h264

import (
	"encoding/base64"
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
)

const (
	NALUTypePFrame = 1
	NALUTypeIFrame = 5
	NALUTypeSPS    = 7
	NALUTypePPS    = 8

	PayloadTypeAVC = 255
)

func NALUType(b []byte) byte {
	return b[4] & 0x1F
}

func EncodeAVC(raw []byte) (avc []byte) {
	avc = make([]byte, len(raw)+4)
	binary.BigEndian.PutUint32(avc, uint32(len(raw)))
	copy(avc[4:], raw)
	return
}

func IsAVC(codec *streamer.Codec) bool {
	return codec.PayloadType == PayloadTypeAVC
}

func GetParameterSet(fmtp string) (sps, pps []byte) {
	if fmtp == "" {
		return
	}

	s := streamer.Between(fmtp, "sprop-parameter-sets=", ";")
	if s == "" {
		return
	}

	i := strings.IndexByte(s, ',')
	if i < 0 {
		return
	}

	sps, _ = base64.StdEncoding.DecodeString(s[:i])
	pps, _ = base64.StdEncoding.DecodeString(s[i+1:])

	return
}
