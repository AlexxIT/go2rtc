package wav

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

const FourCC = "RIFF"

func Open(r io.Reader) (*Producer, error) {
	// https://en.wikipedia.org/wiki/WAV
	// https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/WAVE.html
	rd := bufio.NewReaderSize(r, core.BufferSize)

	// skip Master RIFF chunk
	if _, err := rd.Discard(12); err != nil {
		return nil, err
	}

	codec := &core.Codec{}

	for {
		chunkID, data, err := readChunk(rd)
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

			codec.Channels = uint16(data[2])
			codec.ClockRate = binary.LittleEndian.Uint32(data[4:])
		}
	}

	if codec.Name == "" {
		return nil, errors.New("waw: unsupported codec")
	}

	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{codec},
		},
	}
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "wav",
			Medias:     medias,
			Transport:  r,
		},
		rd: rd,
	}, nil
}

type Producer struct {
	core.Connection
	rd *bufio.Reader
}

func (c *Producer) Start() error {
	var seq uint16
	var ts uint32

	const PacketSize = 0.040 * 8000 // 40ms

	for {
		payload := make([]byte, PacketSize)
		if _, err := io.ReadFull(c.rd, payload); err != nil {
			return err
		}

		c.Recv += PacketSize

		if len(c.Receivers) == 0 {
			continue
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,
				SequenceNumber: seq,
				Timestamp:      ts,
			},
			Payload: payload,
		}
		c.Receivers[0].WriteRTP(pkt)

		seq++
		ts += PacketSize
	}
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
