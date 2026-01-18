package wav

import (
	"bufio"
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

	codec, err := ReadHeader(r)
	if err != nil {
		return nil, err
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
