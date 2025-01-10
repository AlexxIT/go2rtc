package device

const (
	V4L2_PIX_FMT_YUYV  = 'Y' | 'U'<<8 | 'Y'<<16 | 'V'<<24
	V4L2_PIX_FMT_MJPEG = 'M' | 'J'<<8 | 'P'<<16 | 'G'<<24
)

type Format struct {
	FourCC uint32
	Name   string
	FFmpeg string
}

var Formats = []Format{
	{V4L2_PIX_FMT_YUYV, "YUV 4:2:2", "yuyv422"},
	{V4L2_PIX_FMT_MJPEG, "Motion-JPEG", "mjpeg"},
}

// YUYV2YUV convert packed YUV to planar YUV
func YUYV2YUV(dst, src []byte) {
	n := len(src)
	i0 := 0
	iy := 0
	iu := n / 2
	iv := n / 4 * 3
	for i0 < n {
		dst[iy] = src[i0]
		i0++
		iy++
		dst[iu] = src[i0]
		i0++
		iu++
		dst[iy] = src[i0]
		i0++
		iy++
		dst[iv] = src[i0]
		i0++
		iv++
	}
}
