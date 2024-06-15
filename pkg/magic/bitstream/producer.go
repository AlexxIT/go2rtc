package bitstream

import (
	"encoding/hex"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *core.ReadBuffer
}

func Open(r io.Reader) (*Producer, error) {
	rd := core.NewReadBuffer(r)

	buf, err := rd.Peek(256)
	if err != nil {
		return nil, err
	}

	buf = annexb.EncodeToAVCC(buf, false) // won't break original buffer

	var codec *core.Codec
	var format string

	switch {
	case h264.NALUType(buf) == h264.NALUTypeSPS:
		codec = h264.AVCCToCodec(buf)
		format = "h264"
	case h265.NALUType(buf) == h265.NALUTypeVPS:
		codec = h265.AVCCToCodec(buf)
		format = "hevc"
	default:
		return nil, errors.New("bitstream: unsupported header: " + hex.EncodeToString(buf[:8]))
	}

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{codec},
		},
	}
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: format,
			Medias:     medias,
			Transport:  r,
		},
		rd: rd,
	}, nil
}

func (c *Producer) Start() error {
	var buf []byte

	b := make([]byte, core.BufferSize)
	for {
		n, err := c.rd.Read(b)
		if err != nil {
			return err
		}

		c.Recv += n

		buf = append(buf, b[:n]...)

		for {
			i := annexb.IndexFrame(buf)
			if i < 0 {
				break
			}

			if len(c.Receivers) > 0 {
				pkt := &rtp.Packet{
					Header:  rtp.Header{Timestamp: core.Now90000()},
					Payload: annexb.EncodeToAVCC(buf[:i], true),
				}
				c.Receivers[0].WriteRTP(pkt)

				//log.Printf("[AVC] %v, len: %d", h264.Types(pkt.Payload), len(pkt.Payload))
			}

			buf = buf[i:]
		}
	}
}
