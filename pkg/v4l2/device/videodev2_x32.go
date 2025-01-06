//go:build 386 || arm

package device

// https://github.com/torvalds/linux/blob/master/include/uapi/linux/videodev2.h

const (
	VIDIOC_QUERYCAP = 0x80685600
	VIDIOC_ENUM_FMT = 0xc0405602
	VIDIOC_G_FMT    = 0xc0cc5604
	VIDIOC_S_FMT    = 0xc0cc5605
	VIDIOC_REQBUFS  = 0xc0145608
	VIDIOC_QUERYBUF = 0xc0445609

	VIDIOC_QBUF      = 0xc044560f
	VIDIOC_DQBUF     = 0xc0445611
	VIDIOC_STREAMON  = 0x40045612
	VIDIOC_STREAMOFF = 0x40045613
	VIDIOC_G_PARM    = 0xc0cc5615
	VIDIOC_S_PARM    = 0xc0cc5616

	VIDIOC_ENUM_FRAMESIZES     = 0xc02c564a
	VIDIOC_ENUM_FRAMEINTERVALS = 0xc034564b
)

const (
	V4L2_BUF_TYPE_VIDEO_CAPTURE = 1
	V4L2_COLORSPACE_DEFAULT     = 0
	V4L2_FIELD_NONE             = 1
	V4L2_FRMIVAL_TYPE_DISCRETE  = 1
	V4L2_FRMSIZE_TYPE_DISCRETE  = 1
	V4L2_MEMORY_MMAP            = 1
)

type v4l2_capability struct {
	driver       [16]byte
	card         [32]byte
	bus_info     [32]byte
	version      uint32
	capabilities uint32
	device_caps  uint32
	reserved     [3]uint32
}

type v4l2_format struct {
	typ uint32
	fmt v4l2_pix_format
}

type v4l2_pix_format struct {
	width        uint32 // 0
	height       uint32 // 4
	pixelformat  uint32 // 8
	field        uint32 // 12
	bytesperline uint32 // 16
	sizeimage    uint32 // 20
	colorspace   uint32 // 24
	priv         uint32 // 28
	flags        uint32 // 32
	ycbcr_enc    uint32 // 36
	quantization uint32 // 40
	xfer_func    uint32 // 44

	_ [152]byte // 48
}

type v4l2_streamparm struct {
	typ     uint32
	capture v4l2_captureparm
}

type v4l2_captureparm struct {
	capability   uint32     // 0
	capturemode  uint32     // 4
	timeperframe v4l2_fract // 8
	extendedmode uint32     // 16
	readbuffers  uint32     // 20

	_ [176]byte // 24
}

type v4l2_fract struct {
	numerator   uint32
	denominator uint32
}

type v4l2_requestbuffers struct {
	count        uint32
	typ          uint32
	memory       uint32
	capabilities uint32
	flags        uint8
	reserved     [3]uint8
}

type v4l2_buffer struct {
	index     uint32        // 0
	typ       uint32        // 4
	bytesused uint32        // 8
	flags     uint32        // 12
	field     uint32        // 16
	_         [8]byte       // 20
	timecode  v4l2_timecode // 28
	sequence  uint32        // 44
	memory    uint32        // 48
	offset    uint32        // 52
	length    uint32        // 56
	_         [8]byte       // 60
}

type v4l2_timecode struct {
	typ      uint32
	flags    uint32
	frames   uint8
	seconds  uint8
	minutes  uint8
	hours    uint8
	userbits [4]uint8
}

type v4l2_fmtdesc struct {
	index       uint32
	typ         uint32
	flags       uint32
	description [32]byte
	pixelformat uint32
	mbus_code   uint32
	reserved    [3]uint32
}

type v4l2_frmsizeenum struct {
	index        uint32                // 0
	pixel_format uint32                // 4
	typ          uint32                // 8
	discrete     v4l2_frmsize_discrete // 12
	_            [24]byte
}

type v4l2_frmsize_discrete struct {
	width  uint32
	height uint32
}

type v4l2_frmivalenum struct {
	index        uint32
	pixel_format uint32
	width        uint32
	height       uint32
	typ          uint32
	discrete     v4l2_fract
	_            [24]byte
}
