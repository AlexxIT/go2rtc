package h264

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/core"
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

// GetProfileLevelID - get profile from fmtp line
// Some devices won't play video with high level, so limit max profile and max level.
// And return some profile even if fmtp line is empty.
func GetProfileLevelID(fmtp string) string {
	// avc1.640029 - H.264 high 4.1 (Chromecast 1st and 2nd Gen)
	profile := byte(0x64)
	capab := byte(0)
	level := byte(0x29)

	if fmtp != "" {
		var conf []byte
		// some cameras has wrong profile-level-id
		// https://github.com/AlexxIT/go2rtc/issues/155
		if s := core.Between(fmtp, "sprop-parameter-sets=", ","); s != "" {
			if sps, _ := base64.StdEncoding.DecodeString(s); len(sps) >= 4 {
				conf = sps[1:4]
			}
		} else if s = core.Between(fmtp, "profile-level-id=", ";"); s != "" {
			conf, _ = hex.DecodeString(s)
		}

		if conf != nil {
			if conf[0] < profile {
				profile = conf[0]
				capab = conf[1]
			}
			if conf[2] < level {
				level = conf[2]
			}
		}
	}

	return fmt.Sprintf("%02X%02X%02X", profile, capab, level)
}

func GetParameterSet(fmtp string) (sps, pps []byte) {
	if fmtp == "" {
		return
	}

	s := core.Between(fmtp, "sprop-parameter-sets=", ";")
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

// GetFmtpLine from SPS+PPS+IFrame in AVC format
func GetFmtpLine(avc []byte) string {
	s := "packetization-mode=1"

	for {
		size := 4 + int(binary.BigEndian.Uint32(avc))

		switch NALUType(avc) {
		case NALUTypeSPS:
			s += ";profile-level-id=" + hex.EncodeToString(avc[5:8])
			s += ";sprop-parameter-sets=" + base64.StdEncoding.EncodeToString(avc[4:size])
		case NALUTypePPS:
			s += "," + base64.StdEncoding.EncodeToString(avc[4:size])
		}

		if size < len(avc) {
			avc = avc[size:]
		} else {
			return s
		}
	}
}
