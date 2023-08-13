package avc

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

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
