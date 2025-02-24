package mjpeg

import (
	"bytes"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *core.ReadBuffer
}

func Open(rd io.Reader) (*Producer, error) {
	medias := []*core.Media{
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
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "mjpeg",
			Medias:     medias,
			Transport:  rd,
		},
		rd: core.NewReadBuffer(rd),
	}, nil
}

func (c *Producer) Start() error {
	var buf []byte                     // total bufer
	b := make([]byte, core.BufferSize) // reading buffer

	for {
		// one JPEG end and next start
		i := bytes.Index(buf, []byte{0xFF, 0xD9, 0xFF, 0xD8})
		if i < 0 {
			n, err := c.rd.Read(b)
			if err != nil {
				return err
			}

			c.Recv += n

			buf = append(buf, b[:n]...)

			// if we receive frame
			if n >= 2 && b[n-2] == 0xFF && b[n-1] == 0xD9 {
				i = len(buf)
			} else {
				continue
			}
		} else {
			i += 2
		}

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: buf[:i],
		}
		c.Receivers[0].WriteRTP(pkt)

		//log.Printf("[mjpeg] ts=%d size=%d", pkt.Header.Timestamp, len(pkt.Payload))

		buf = buf[i:]
	}
}
