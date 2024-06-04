package aac

import (
	"encoding/hex"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

const (
	TypeAACMain = 1
	TypeAACLC   = 2  // Low Complexity
	TypeAACLD   = 23 // Low Delay (48000, 44100, 32000, 24000, 22050)
	TypeESCAPE  = 31
	TypeAACELD  = 39 // Enhanced Low Delay

	AUTime = 1024

	// FMTP streamtype=5 - audio stream
	FMTP = "streamtype=5;profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config="
)

var sampleRates = [16]uint32{
	96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350,
	0, 0, 0, // protection from request sampleRates[15]
}

func ConfigToCodec(conf []byte) *core.Codec {
	// https://en.wikipedia.org/wiki/MPEG-4_Part_3#MPEG-4_Audio_Object_Types
	rd := bits.NewReader(conf)

	codec := &core.Codec{
		FmtpLine:    FMTP + hex.EncodeToString(conf),
		PayloadType: core.PayloadTypeRAW,
	}

	objType := rd.ReadBits(5)
	if objType == TypeESCAPE {
		objType = 32 + rd.ReadBits(6)
	}

	switch objType {
	case TypeAACLC, TypeAACLD, TypeAACELD:
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
	} else {
		sampleRate = sampleRates[sampleFreqIdx]
	}

	channels = rd.ReadBits8(4)
	return
}

func EncodeConfig(objType byte, sampleRate uint32, channels byte, shortFrame bool) []byte {
	wr := bits.NewWriter(nil)

	if objType < TypeESCAPE {
		wr.WriteBits8(objType, 5)
	} else {
		wr.WriteBits8(TypeESCAPE, 5)
		wr.WriteBits8(objType-32, 6)
	}

	i := indexUint32(sampleRates[:], sampleRate)
	if i >= 0 {
		wr.WriteBits8(byte(i), 4)
	} else {
		wr.WriteBits8(0xF, 4)
		wr.WriteBits(sampleRate, 24)
	}

	wr.WriteBits8(channels, 4)

	switch objType {
	case TypeAACLD:
		// https://github.com/FFmpeg/FFmpeg/blob/67d392b97941bb51fb7af3a3c9387f5ab895fa46/libavcodec/aacdec_template.c#L841
		wr.WriteBool(shortFrame)
		wr.WriteBit(0)      // dependsOnCoreCoder
		wr.WriteBit(0)      // extension_flag
		wr.WriteBits8(0, 2) // ep_config
	case TypeAACELD:
		// https://github.com/FFmpeg/FFmpeg/blob/67d392b97941bb51fb7af3a3c9387f5ab895fa46/libavcodec/aacdec_template.c#L922
		wr.WriteBool(shortFrame)
		wr.WriteBits8(0, 3) // res_flags
		wr.WriteBit(0)      // ldSbrPresentFlag
		wr.WriteBits8(0, 4) // ELDEXT_TERM
		wr.WriteBits8(0, 2) // ep_config
	}

	return wr.Bytes()
}

func indexUint32(s []uint32, v uint32) int {
	for i := range s {
		if v == s[i] {
			return i
		}
	}
	return -1
}
