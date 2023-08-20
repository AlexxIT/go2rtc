package aac

import (
	"encoding/hex"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

const (
	TypeAACMain = 1
	TypeAACLC   = 2
	TypeESCAPE  = 31

	AUTime = 1024
)

// streamtype=5 - audio stream
const fmtp = "streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config="

var sampleRates = []uint32{
	96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350,
	0, 0, 0, // protection from request sampleRates[15]
}

func ConfigToCodec(conf []byte) *core.Codec {
	// https://en.wikipedia.org/wiki/MPEG-4_Part_3#MPEG-4_Audio_Object_Types
	rd := bits.NewReader(conf)

	codec := &core.Codec{
		FmtpLine:    fmtp + hex.EncodeToString(conf),
		PayloadType: core.PayloadTypeRAW,
	}

	objType := rd.ReadBits(5)
	if objType == TypeESCAPE {
		objType = 32 + rd.ReadBits(6)
	}

	switch objType {
	case TypeAACLC:
		codec.Name = core.CodecAAC
	default:
		codec.Name = fmt.Sprintf("AAC-%X", objType)
	}

	if sampleRateIdx := rd.ReadBits8(4); sampleRateIdx < 0x0F {
		codec.ClockRate = sampleRates[sampleRateIdx]
	} else {
		codec.ClockRate = rd.ReadBits(24)
	}

	codec.Channels = rd.ReadBits16(4)

	return codec
}

func DecodeConfig(b []byte) (objType, sampleFreqIdx, channels byte, sampleRate uint32) {
	rd := bits.NewReader(b)

	objType = rd.ReadBits8(5)
	if objType == 0b11111 {
		objType = 32 + rd.ReadBits8(6)
	}

	sampleFreqIdx = rd.ReadBits8(4)
	if sampleFreqIdx == 0b1111 {
		sampleRate = rd.ReadBits(24)
	}

	channels = rd.ReadBits8(4)
	return
}
