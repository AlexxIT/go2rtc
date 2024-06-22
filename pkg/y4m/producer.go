package y4m

import (
	"bufio"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func Open(r io.Reader) (*Producer, error) {
	rd := bufio.NewReaderSize(r, core.BufferSize)
	b, err := rd.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	b = b[:len(b)-1] // remove \n

	fmtp := ParseHeader(b)

	if GetSize(fmtp) == 0 {
		return nil, errors.New("y4m: unsupported format: " + string(b))
	}

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:        core.CodecRAW,
					ClockRate:   90000,
					FmtpLine:    fmtp,
					PayloadType: core.PayloadTypeRAW,
				},
			},
		},
	}
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "yuv4mpegpipe",
			Medias:     medias,
			SDP:        string(b),
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
	size := GetSize(c.Medias[0].Codecs[0].FmtpLine)

	for {
		if _, err := c.rd.Discard(len(frameHdr)); err != nil {
			return err
		}

		frame := make([]byte, size)
		if _, err := io.ReadFull(c.rd, frame); err != nil {
			return err
		}

		c.Recv += size

		if len(c.Receivers) == 0 {
			continue
		}

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: frame,
		}
		c.Receivers[0].WriteRTP(pkt)
	}
}
