package h265

import (
	"encoding/base64"
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
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
	return (b[4] >> 1) & 0x3F
}

func IsKeyframe(b []byte) bool {
	for {
		switch NALUType(b) {
		case NALUTypePFrame:
			return false
		case NALUTypeIFrame, NALUTypeIFrame2, NALUTypeIFrame3:
			return true
		}

		size := int(binary.BigEndian.Uint32(b)) + 4
		if size < len(b) {
			b = b[size:]
			continue
		} else {
			return false
		}
	}
}

func Types(data []byte) []byte {
	var types []byte
	for {
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

	s := streamer.Between(fmtp, "sprop-vps=", ";")
	vps, _ = base64.StdEncoding.DecodeString(s)

	s = streamer.Between(fmtp, "sprop-sps=", ";")
	sps, _ = base64.StdEncoding.DecodeString(s)

	s = streamer.Between(fmtp, "sprop-pps=", ";")
	pps, _ = base64.StdEncoding.DecodeString(s)

	return
}
