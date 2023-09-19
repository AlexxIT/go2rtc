// Package h264 - MPEG4 format related functions
package h264

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// DecodeConfig - extract profile, SPS and PPS from MPEG4 config
func DecodeConfig(conf []byte) (profile []byte, sps []byte, pps []byte) {
	if len(conf) < 6 || conf[0] != 1 {
		return
	}

	profile = conf[1:4]

	count := conf[5] & 0x1F
	conf = conf[6:]
	for i := byte(0); i < count; i++ {
		if len(conf) < 2 {
			return
		}
		size := 2 + int(binary.BigEndian.Uint16(conf))
		if len(conf) < size {
			return
		}
		if sps == nil {
			sps = conf[2:size]
		}
		conf = conf[size:]
	}

	count = conf[0]
	conf = conf[1:]
	for i := byte(0); i < count; i++ {
		if len(conf) < 2 {
			return
		}
		size := 2 + int(binary.BigEndian.Uint16(conf))
		if len(conf) < size {
			return
		}
		if pps == nil {
			pps = conf[2:size]
		}
		conf = conf[size:]
	}

	return
}

func EncodeConfig(sps, pps []byte) []byte {
	spsSize := uint16(len(sps))
	ppsSize := uint16(len(pps))

	buf := make([]byte, 5+3+spsSize+3+ppsSize)
	buf[0] = 1
	copy(buf[1:], sps[1:4]) // profile
	buf[4] = 3 | 0xFC       // ? LengthSizeMinusOne

	b := buf[5:]
	_ = b[3]
	b[0] = 1 | 0xE0 // ? sps count
	binary.BigEndian.PutUint16(b[1:], spsSize)
	copy(b[3:], sps)

	b = buf[5+3+spsSize:]
	_ = b[3]
	b[0] = 1 // pps count
	binary.BigEndian.PutUint16(b[1:], ppsSize)
	copy(b[3:], pps)

	return buf
}

func ConfigToCodec(conf []byte) *core.Codec {
	buf := bytes.NewBufferString("packetization-mode=1")

	profile, sps, pps := DecodeConfig(conf)
	if profile != nil {
		buf.WriteString(";profile-level-id=")
		buf.WriteString(hex.EncodeToString(profile))
	}
	if sps != nil && pps != nil {
		buf.WriteString(";sprop-parameter-sets=")
		buf.WriteString(base64.StdEncoding.EncodeToString(sps))
		buf.WriteString(",")
		buf.WriteString(base64.StdEncoding.EncodeToString(pps))
	}

	return &core.Codec{
		Name:        core.CodecH264,
		ClockRate:   90000,
		FmtpLine:    buf.String(),
		PayloadType: core.PayloadTypeRAW,
	}
}
