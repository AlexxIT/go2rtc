package device

const (
	V4L2_PIX_FMT_YUYV  = 'Y' | 'U'<<8 | 'Y'<<16 | 'V'<<24
	V4L2_PIX_FMT_NV12  = 'N' | 'V'<<8 | '1'<<16 | '2'<<24
	V4L2_PIX_FMT_MJPEG = 'M' | 'J'<<8 | 'P'<<16 | 'G'<<24
	V4L2_PIX_FMT_H264  = 'H' | '2'<<8 | '6'<<16 | '4'<<24
	V4L2_PIX_FMT_HEVC  = 'H' | 'E'<<8 | 'V'<<16 | 'C'<<24
)

type Format struct {
	FourCC uint32
	Name   string
	FFmpeg string
}

var Formats = []Format{
	{V4L2_PIX_FMT_YUYV, "YUV 4:2:2", "yuyv422"},
	{V4L2_PIX_FMT_NV12, "Y/UV 4:2:0", "nv12"},
	{V4L2_PIX_FMT_MJPEG, "Motion-JPEG", "mjpeg"},
	{V4L2_PIX_FMT_H264, "H.264", "h264"},
	{V4L2_PIX_FMT_HEVC, "HEVC", "hevc"},
}

func YUYVtoYUV(dst, src []byte) {
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

func NV12toYUV(dst, src []byte) {
	n := len(src)
	k := n / 6
	i0 := k * 4
	iu := i0
	iv := i0 + k
	copy(dst, src[:i0]) // copy Y
	for i0 < n {
		dst[iu] = src[i0]
		i0++
		iu++
		dst[iv] = src[i0]
		i0++
		iv++
	}
}
