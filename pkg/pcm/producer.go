package pcm

import (
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd io.Reader
}

func Open(rd io.Reader) (*Producer, error) {
	medias := []*core.Media{
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{Name: core.CodecPCMU, ClockRate: 8000},
			},
		},
	}
	return &Producer{
		core.Connection{
			ID:         core.NewID(),
			FormatName: "pcm",
			Medias:     medias,
			Transport:  rd,
		},
		rd,
	}, nil
}

func (c *Producer) Start() error {
	for {
		payload := make([]byte, 1024)
		if _, err := io.ReadFull(c.rd, payload); err != nil {
			return err
		}

		c.Recv += 1024

		if len(c.Receivers) == 0 {
			continue
		}

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: payload,
		}
		c.Receivers[0].WriteRTP(pkt)
	}
}
