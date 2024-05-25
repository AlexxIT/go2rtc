package y4m

import (
	"bufio"
	"bytes"
	"errors"
	"image"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

const FourCC = "YUV4"

func Open(r io.Reader) (*Producer, error) {
	rd := bufio.NewReaderSize(r, core.BufferSize)
	b, err := rd.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	b = b[:len(b)-1] // remove \n

	sdp := string(b)
	var fmtp string

	for b != nil {
		// YUV4MPEG2 W1280 H720 F24:1 Ip A1:1 C420mpeg2 XYSCSS=420MPEG2
		// https://manned.org/yuv4mpeg.5
		// https://github.com/FFmpeg/FFmpeg/blob/master/libavformat/yuv4mpegenc.c
		key := b[0]
		var value string
		if i := bytes.IndexByte(b, ' '); i > 0 {
			value = string(b[1:i])
			b = b[i+1:]
		} else {
			value = string(b[1:])
			b = nil
		}

		switch key {
		case 'W':
			fmtp = "width=" + value
		case 'H':
			fmtp += ";height=" + value
		case 'C':
			fmtp += ";colorspace=" + value
		}
	}

	if GetSize(fmtp) == 0 {
		return nil, errors.New("y4m: unsupported format: " + sdp)
	}

	prod := &Producer{rd: rd, cl: r.(io.Closer)}
	prod.Type = "YUV4MPEG2 producer"
	prod.SDP = sdp
	prod.Medias = []*core.Media{
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

	return prod, nil
}

type Producer struct {
	core.SuperProducer
	rd *bufio.Reader
	cl io.Closer
}

func (c *Producer) Start() error {
	size := GetSize(c.Medias[0].Codecs[0].FmtpLine)

	for {
		// FRAME\n
		if _, err := c.rd.Discard(6); err != nil {
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

func (c *Producer) Stop() error {
	_ = c.SuperProducer.Close()
	return c.cl.Close()
}

func GetSize(fmtp string) int {
	w := core.Atoi(core.Between(fmtp, "width=", ";"))
	h := core.Atoi(core.Between(fmtp, "height=", ";"))

	switch core.Between(fmtp, "colorspace=", ";") {
	case "mono":
		return w * h
	case "420mpeg2", "420jpeg":
		return w * h * 3 / 2
	case "422":
		return w * h * 2
	case "444":
		return w * h * 3
	}

	return 0
}

func NewImage(fmtp string) func(frame []byte) image.Image {
	w := core.Atoi(core.Between(fmtp, "width=", ";"))
	h := core.Atoi(core.Between(fmtp, "height=", ";"))
	rect := image.Rect(0, 0, w, h)

	switch core.Between(fmtp, "colorspace=", ";") {
	case "mono":
		return func(frame []byte) image.Image {
			return &image.Gray{
				Pix:    frame,
				Stride: w,
				Rect:   rect,
			}
		}
	case "420mpeg2", "420jpeg":
		i1 := w * h
		i2 := i1 + i1/4
		i3 := i2 + i1/4

		return func(frame []byte) image.Image {
			return &image.YCbCr{
				Y:              frame[:i1],
				Cb:             frame[i1:i2],
				Cr:             frame[i2:i3],
				YStride:        w,
				CStride:        w / 2,
				SubsampleRatio: image.YCbCrSubsampleRatio420,
				Rect:           rect,
			}
		}
	case "422":
		i1 := w * h
		i2 := i1 + i1/2
		i3 := i2 + i1/2

		return func(frame []byte) image.Image {
			return &image.YCbCr{
				Y:              frame[:i1],
				Cb:             frame[i1:i2],
				Cr:             frame[i2:i3],
				YStride:        w,
				CStride:        w / 2,
				SubsampleRatio: image.YCbCrSubsampleRatio422,
				Rect:           rect,
			}
		}
	case "444":
		i1 := w * h
		i2 := i1 + i1
		i3 := i2 + i1

		return func(frame []byte) image.Image {
			return &image.YCbCr{
				Y:              frame[:i1],
				Cb:             frame[i1:i2],
				Cr:             frame[i2:i3],
				YStride:        w,
				CStride:        w,
				SubsampleRatio: image.YCbCrSubsampleRatio444,
				Rect:           rect,
			}
		}
	}

	return nil
}
