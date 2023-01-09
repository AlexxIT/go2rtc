package h264

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
)

const (
	NALUTypePFrame = 1 // Coded slice of a non-IDR picture
	NALUTypeIFrame = 5 // Coded slice of an IDR picture
	NALUTypeSEI    = 6 // Supplemental enhancement information (SEI)
	NALUTypeSPS    = 7 // Sequence parameter set
	NALUTypePPS    = 8 // Picture parameter set
	NALUTypeAUD    = 9 // Access unit delimiter
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

	// some cameras has wrong profile-level-id
	// https://github.com/AlexxIT/go2rtc/issues/155
	if s := streamer.Between(fmtp, "sprop-parameter-sets=", ","); s != "" {
		sps, _ := base64.StdEncoding.DecodeString(s)
		if len(sps) >= 4 {
			return fmt.Sprintf("%06X", sps[1:4])
		}
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
