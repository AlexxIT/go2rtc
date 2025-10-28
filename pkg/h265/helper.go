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

	s := core.Between(fmtp, "sprop-vps=", ";")
	vps, _ = base64.StdEncoding.DecodeString(s)

	s = core.Between(fmtp, "sprop-sps=", ";")
	sps, _ = base64.StdEncoding.DecodeString(s)

	s = core.Between(fmtp, "sprop-pps=", ";")
	pps, _ = base64.StdEncoding.DecodeString(s)

	return
}

func ContainsParameterSets(payload []byte) bool {
	types := Types(payload)
	hasVPS, hasSPS, hasPPS := false, false, false

	for _, nalType := range types {
		switch nalType {
		case NALUTypeVPS:
			hasVPS = true
		case NALUTypeSPS:
			hasSPS = true
		case NALUTypePPS:
			hasPPS = true
		}
	}

	return hasVPS && hasSPS && hasPPS
}

func GetFmtpLine(avcc []byte) string {
	var vps, sps, pps []byte

	for {
		size := 4 + int(binary.BigEndian.Uint32(avcc))

		switch NALUType(avcc) {
		case NALUTypeVPS:
			vps = avcc[4:size]
		case NALUTypeSPS:
			sps = avcc[4:size]
		case NALUTypePPS:
			pps = avcc[4:size]
		}

		if size < len(avcc) {
			avcc = avcc[size:]
		} else {
			break
		}
	}

	if len(vps) == 0 || len(sps) == 0 || len(pps) == 0 {
		return ""
	}

	fmtp := "sprop-vps=" + base64.StdEncoding.EncodeToString(vps)
	fmtp += ";sprop-sps=" + base64.StdEncoding.EncodeToString(sps)
	fmtp += ";sprop-pps=" + base64.StdEncoding.EncodeToString(pps)

	return fmtp
}

func UpdateFmtpLine(codec *core.Codec, payload []byte) {
	if !ContainsParameterSets(payload) {
		return
	}

	newFmtpLine := GetFmtpLine(payload)
	if newFmtpLine != "" {
		codec.FmtpLine = newFmtpLine
	}
}