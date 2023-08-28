package multipart

import (
	"bufio"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.SuperProducer
	closer io.Closer
	reader *bufio.Reader
}

func Open(rd io.Reader) (*Producer, error) {
	prod := &Producer{
		closer: rd.(io.Closer),
		reader: bufio.NewReader(rd),
	}
	prod.Medias = []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:        core.CodecJPEG,
					ClockRate:   90000,
					PayloadType: core.PayloadTypeRAW,
				},
			},
		},
	}
	prod.Type = "Multipart producer"
	return prod, nil
}

func (c *Producer) Start() error {
	if len(c.Receivers) != 1 {
		return errors.New("mjpeg: no receivers")
	}

	mjpeg := c.Receivers[0]

	for {
		_, body, err := Next(c.reader)
		if err != nil {
			return err
		}

		c.Recv += len(body)

		if mjpeg != nil {
			packet := &rtp.Packet{
				Header:  rtp.Header{Timestamp: core.Now90000()},
				Payload: body,
			}
			mjpeg.WriteRTP(packet)
		}
	}
}

func (c *Producer) Stop() error {
	_ = c.SuperProducer.Close()
	return c.closer.Close()
}
