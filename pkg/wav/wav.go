package wav

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func Header(codec *core.Codec) []byte {
	var fmt, size, extra byte

	switch codec.Name {
	case core.CodecPCML:
		fmt = 1
		size = 2
	case core.CodecPCMA:
		fmt = 6
		size = 1
		extra = 2
	case core.CodecPCMU:
		fmt = 7
		size = 1
		extra = 2
	default:
		return nil
	}

	channels := byte(codec.Channels)
	if channels == 0 {
		channels = 1
	}

	b := make([]byte, 0, 46) // cap with extra
	b = append(b, "RIFF\xFF\xFF\xFF\xFFWAVEfmt "...)

	b = append(b, 0x10+extra, 0, 0, 0)
	b = append(b, fmt, 0)
	b = append(b, channels, 0)
	b = binary.LittleEndian.AppendUint32(b, codec.ClockRate)
	b = binary.LittleEndian.AppendUint32(b, uint32(size*channels)*codec.ClockRate)
	b = append(b, size*channels, 0)
	b = append(b, size*8, 0)
	if extra > 0 {
		b = append(b, 0, 0) // ExtraParamSize (if PCM, then doesn't exist)
	}

	b = append(b, "data\xFF\xFF\xFF\xFF"...)

	return b
}
