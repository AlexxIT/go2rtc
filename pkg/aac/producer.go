package aac

import (
	"bufio"
	"encoding/binary"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.SuperProducer
	rd *bufio.Reader
	cl io.Closer
}

func Open(r io.Reader) (*Producer, error) {
	rd := bufio.NewReader(r)

	b, err := rd.Peek(8)
	if err != nil {
		return nil, err
	}

	codec := ADTSToCodec(b)

	prod := &Producer{rd: rd, cl: r.(io.Closer)}
	prod.Type = "ADTS producer"
	prod.Medias = []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{codec},
		},
	}
	return prod, nil
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

func (c *Producer) Stop() error {
	_ = c.SuperProducer.Close()
	return c.cl.Close()
}
