package y4m

import (
	"image"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const FourCC = "YUV4"

const frameHdr = "FRAME\n"

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
