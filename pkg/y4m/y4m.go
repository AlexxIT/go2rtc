package y4m

import (
	"bytes"
	"image"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const FourCC = "YUV4"

const frameHdr = "FRAME\n"

func ParseHeader(b []byte) (fmtp string) {
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
	return
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
