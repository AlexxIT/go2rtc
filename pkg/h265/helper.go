package h265

import (
	"encoding/base64"
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const (
	NALUTypePFrame    = 1
	NALUTypeIFrame    = 19
	NALUTypeIFrame2   = 20
	NALUTypeIFrame3   = 21
	NALUTypeVPS       = 32
	NALUTypeSPS       = 33
	NALUTypePPS       = 34
	NALUTypePrefixSEI = 39
	NALUTypeSuffixSEI = 40
	NALUTypeFU        = 49
)

func NALUType(b []byte) byte {
	if len(b) < 5 {
		return 0
	}
	return (b[4] >> 1) & 0x3F
}

func IsKeyframe(b []byte) bool {
	hasVPS := false
	for len(b) >= 5 {
		switch NALUType(b) {
		case NALUTypeIFrame, NALUTypeIFrame2, NALUTypeIFrame3:
			return true
		case NALUTypeVPS:
			hasVPS = true
		case NALUTypePFrame:
			return hasVPS
		}

		size := int(binary.BigEndian.Uint32(b)) + 4
		if size < len(b) {
			b = b[size:]
			continue
		} else {
			return hasVPS
		}
	}
	return hasVPS
}

func Types(data []byte) []byte {
	var types []byte
	for len(data) >= 5 {
		types = append(types, NALUType(data))

		size := 4 + int(binary.BigEndian.Uint32(data))
		if size < len(data) {
			data = data[size:]
		} else {
			break
		}
	}
	return types
}

func GetParameterSet(fmtp string) (vps, sps, pps []byte) {
	if fmtp == "" {
		return
	}

	s := core.Between(fmtp, "sprop-vps=", ";")
	vps, _ = base64.StdEncoding.DecodeString(s)

	s = core.Between(fmtp, "sprop-sps=", ";")
	sps, _ = base64.StdEncoding.DecodeString(s)

	s = core.Between(fmtp, "sprop-pps=", ";")
	pps, _ = base64.StdEncoding.DecodeString(s)

	return
}
