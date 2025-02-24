package aac

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *bufio.Reader
}

func Open(r io.Reader) (*Producer, error) {
	rd := bufio.NewReader(r)

	b, err := rd.Peek(8)
	if err != nil {
		return nil, err
	}

	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{ADTSToCodec(b)},
		},
	}
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "adts",
			Medias:     medias,
			Transport:  r,
		},
		rd: rd,
	}, nil
}

func (c *Producer) Start() error {
	for {
		b, err := c.rd.Peek(6)
		if err != nil {
			return err
		}

		auSize := ReadADTSSize(b)
		payload := make([]byte, 2+2+auSize)
		if _, err = io.ReadFull(c.rd, payload[4:]); err != nil {
			return err
		}

		c.Recv += int(auSize)

		if len(c.Receivers) == 0 {
			continue
		}

		payload[1] = 16 // header size in bits
		binary.BigEndian.PutUint16(payload[2:], auSize<<3)

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: payload,
		}
		c.Receivers[0].WriteRTP(pkt)
	}
}
