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
	NALUTypeSEI    = 6
	NALUTypeSPS    = 7
	NALUTypePPS    = 8
)

func NALUType(b []byte) byte {
	return b[4] & 0x1F
}

// IsKeyframe - check if any NALU in one AU is Keyframe
func IsKeyframe(b []byte) bool {
	for {
		switch NALUType(b) {
		case NALUTypePFrame:
			return false
		case NALUTypeIFrame:
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

func Join(ps, iframe []byte) []byte {
	b := make([]byte, len(ps)+len(iframe))
	i := copy(b, ps)
	copy(b[i:], iframe)
	return b
}

func GetProfileLevelID(fmtp string) string {
	if fmtp == "" {
		return ""
	}
	return streamer.Between(fmtp, "profile-level-id=", ";")
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
