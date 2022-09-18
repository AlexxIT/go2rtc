package h265

import (
	"encoding/base64"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

const (
	NALUnitTypeIFrame = 19
)

func NALUnitType(b []byte) byte {
	return b[4] >> 1
}

func IsKeyframe(b []byte) bool {
	return NALUnitType(b) == NALUnitTypeIFrame
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
