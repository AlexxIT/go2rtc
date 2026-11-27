// Package h265 - MPEG4 format related functions
package h265

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func DecodeConfig(conf []byte) (profile, vps, sps, pps []byte) {
	if len(conf) < 23 {
		return
	}

	profile = conf[1:4]

	if vps, conf = decodeConfigNALU(conf[23:]); vps == nil {
		return
	}
	if sps, conf = decodeConfigNALU(conf); sps == nil {
		return
	}
	pps, _ = decodeConfigNALU(conf)

	return
}

func decodeConfigNALU(b []byte) (nalu, rest []byte) {
	if len(b) < 5 || binary.BigEndian.Uint16(b[1:3]) != 1 {
		return
	}

	size := int(binary.BigEndian.Uint16(b[3:5]))
	if size > len(b)-5 {
		return
	}

	return b[5 : 5+size], b[5+size:]
}

func EncodeConfig(vps, sps, pps []byte) []byte {
	vpsSize := len(vps)
	spsSize := len(sps)
	ppsSize := len(pps)

	if vpsSize == 0 || vpsSize > 0xFFFF || spsSize < 6 || spsSize > 0xFFFF || ppsSize == 0 || ppsSize > 0xFFFF {
		return nil
	}

	buf := make([]byte, 23+5+vpsSize+5+spsSize+5+ppsSize)

	buf[0] = 1
	copy(buf[1:], sps[3:6]) // profile
	buf[21] = 3             // ?
	buf[22] = 3             // ?

	b := buf[23:]
	_ = b[5]
	b[0] = (vps[0] >> 1) & 0x3F
	binary.BigEndian.PutUint16(b[1:], 1) // VPS count
	binary.BigEndian.PutUint16(b[3:], uint16(vpsSize))
	copy(b[5:], vps)

	b = buf[23+5+vpsSize:]
	_ = b[5]
	b[0] = (sps[0] >> 1) & 0x3F
	binary.BigEndian.PutUint16(b[1:], 1) // SPS count
	binary.BigEndian.PutUint16(b[3:], uint16(spsSize))
	copy(b[5:], sps)

	b = buf[23+5+vpsSize+5+spsSize:]
	_ = b[5]
	b[0] = (pps[0] >> 1) & 0x3F
	binary.BigEndian.PutUint16(b[1:], 1) // PPS count
	binary.BigEndian.PutUint16(b[3:], uint16(ppsSize))
	copy(b[5:], pps)

	return buf
}

func ConfigToCodec(conf []byte) *core.Codec {
	buf := bytes.NewBufferString("profile-id=1")

	_, vps, sps, pps := DecodeConfig(conf)
	if vps != nil {
		buf.WriteString(";sprop-vps=")
		buf.WriteString(base64.StdEncoding.EncodeToString(vps))
	}
	if sps != nil {
		buf.WriteString(";sprop-sps=")
		buf.WriteString(base64.StdEncoding.EncodeToString(sps))
	}
	if pps != nil {
		buf.WriteString(";sprop-pps=")
		buf.WriteString(base64.StdEncoding.EncodeToString(pps))
	}

	return &core.Codec{
		Name:        core.CodecH265,
		ClockRate:   90000,
		FmtpLine:    buf.String(),
		PayloadType: core.PayloadTypeRAW,
	}
}
