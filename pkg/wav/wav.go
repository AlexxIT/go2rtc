package wav

import (
	"encoding/binary"
	"io"

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

func ReadHeader(r io.Reader) (*core.Codec, error) {
	// skip Master RIFF chunk
	if _, err := io.ReadFull(r, make([]byte, 12)); err != nil {
		return nil, err
	}

	var codec core.Codec

	for {
		chunkID, data, err := readChunk(r)
		if err != nil {
			return nil, err
		}

		if chunkID == "data" {
			break
		}

		if chunkID == "fmt " {
			// https://audiocoding.cc/articles/2008-05-22-wav-file-structure/wav_formats.txt
			switch data[0] {
			case 1:
				codec.Name = core.CodecPCML
			case 6:
				codec.Name = core.CodecPCMA
			case 7:
				codec.Name = core.CodecPCMU
			}

			codec.Channels = data[2]
			codec.ClockRate = binary.LittleEndian.Uint32(data[4:])
		}
	}

	return &codec, nil
}

func readChunk(r io.Reader) (chunkID string, data []byte, err error) {
	b := make([]byte, 8)
	if _, err = io.ReadFull(r, b); err != nil {
		return
	}

	if chunkID = string(b[:4]); chunkID != "data" {
		size := binary.LittleEndian.Uint32(b[4:])
		data = make([]byte, size)
		_, err = io.ReadFull(r, data)
	}

	return
}
