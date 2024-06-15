package mpjpeg

import (
	"bufio"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *bufio.Reader
}

func Open(rd io.Reader) (*Producer, error) {
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "mpjpeg", // Multipart JPEG
			Transport:  rd,
			Medias: []*core.Media{
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
			},
		},
	}, nil
}

func (c *Producer) Start() error {
	if len(c.Receivers) != 1 {
		return errors.New("mjpeg: no receivers")
	}

	rd := bufio.NewReader(c.Transport.(io.Reader))

	mjpeg := c.Receivers[0]

	for {
		_, body, err := Next(rd)
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
