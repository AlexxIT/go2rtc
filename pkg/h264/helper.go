package h264

import (
	"encoding/base64"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
)

const (
	NALUTypePFrame = 1
	NALUTypeIFrame = 5
	NALUTypeSPS    = 7
	NALUTypePPS    = 8
)

func NALUType(b []byte) byte {
	return b[4] & 0x1F
}

func IsKeyframe(b []byte) bool {
	return NALUType(b) == NALUTypeIFrame
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
