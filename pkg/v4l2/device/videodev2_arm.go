package device

const (
	VIDIOC_QUERYCAP = 0x80685600
	VIDIOC_ENUM_FMT = 0xc0405602
	VIDIOC_G_FMT    = 0xc0cc5604
	VIDIOC_S_FMT    = 0xc0cc5605
	VIDIOC_REQBUFS  = 0xc0145608
	VIDIOC_QUERYBUF = 0xc0505609

	VIDIOC_QBUF      = 0xc050560f
	VIDIOC_DQBUF     = 0xc0505611
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

type v4l2_capability struct { // size 104
	driver       [16]byte  // offset 0, size 16
	card         [32]byte  // offset 16, size 32
	bus_info     [32]byte  // offset 48, size 32
	version      uint32    // offset 80, size 4
	capabilities uint32    // offset 84, size 4
	device_caps  uint32    // offset 88, size 4
	reserved     [3]uint32 // offset 92, size 12
}

type v4l2_format struct { // size 204
	typ uint32          // offset 0, size 4
	_   [0]byte         // align
	pix v4l2_pix_format // offset 4, size 48
	_   [152]byte       // filler
}

type v4l2_pix_format struct { // size 48
	width        uint32 // offset 0, size 4
	height       uint32 // offset 4, size 4
	pixelformat  uint32 // offset 8, size 4
	field        uint32 // offset 12, size 4
	bytesperline uint32 // offset 16, size 4
	sizeimage    uint32 // offset 20, size 4
	colorspace   uint32 // offset 24, size 4
	priv         uint32 // offset 28, size 4
	flags        uint32 // offset 32, size 4
	ycbcr_enc    uint32 // offset 36, size 4
	quantization uint32 // offset 40, size 4
	xfer_func    uint32 // offset 44, size 4
}

type v4l2_streamparm struct { // size 204
	typ     uint32           // offset 0, size 4
	capture v4l2_captureparm // offset 4, size 40
	_       [160]byte        // filler
}

type v4l2_captureparm struct { // size 40
	capability   uint32     // offset 0, size 4
	capturemode  uint32     // offset 4, size 4
	timeperframe v4l2_fract // offset 8, size 8
	extendedmode uint32     // offset 16, size 4
	readbuffers  uint32     // offset 20, size 4
	reserved     [4]uint32  // offset 24, size 16
}

type v4l2_fract struct { // size 8
	numerator   uint32 // offset 0, size 4
	denominator uint32 // offset 4, size 4
}

type v4l2_requestbuffers struct { // size 20
	count        uint32   // offset 0, size 4
	typ          uint32   // offset 4, size 4
	memory       uint32   // offset 8, size 4
	capabilities uint32   // offset 12, size 4
	flags        uint8    // offset 16, size 1
	reserved     [3]uint8 // offset 17, size 3
}

type v4l2_buffer struct { // size 80
	index     uint32        // offset 0, size 4
	typ       uint32        // offset 4, size 4
	bytesused uint32        // offset 8, size 4
	flags     uint32        // offset 12, size 4
	field     uint32        // offset 16, size 4
	_         [20]byte      // align
	timecode  v4l2_timecode // offset 40, size 16
	sequence  uint32        // offset 56, size 4
	memory    uint32        // offset 60, size 4
	offset    uint32        // offset 64, size 4
	_         [0]byte       // align
	length    uint32        // offset 68, size 4
	_         [8]byte       // filler
}

type v4l2_timecode struct { // size 16
	typ      uint32   // offset 0, size 4
	flags    uint32   // offset 4, size 4
	frames   uint8    // offset 8, size 1
	seconds  uint8    // offset 9, size 1
	minutes  uint8    // offset 10, size 1
	hours    uint8    // offset 11, size 1
	userbits [4]uint8 // offset 12, size 4
}

type v4l2_fmtdesc struct { // size 64
	index       uint32    // offset 0, size 4
	typ         uint32    // offset 4, size 4
	flags       uint32    // offset 8, size 4
	description [32]byte  // offset 12, size 32
	pixelformat uint32    // offset 44, size 4
	mbus_code   uint32    // offset 48, size 4
	reserved    [3]uint32 // offset 52, size 12
}

type v4l2_frmsizeenum struct { // size 44
	index        uint32                // offset 0, size 4
	pixel_format uint32                // offset 4, size 4
	typ          uint32                // offset 8, size 4
	discrete     v4l2_frmsize_discrete // offset 12, size 8
	_            [24]byte              // filler
}

type v4l2_frmsize_discrete struct { // size 8
	width  uint32 // offset 0, size 4
	height uint32 // offset 4, size 4
}

type v4l2_frmivalenum struct { // size 52
	index        uint32     // offset 0, size 4
	pixel_format uint32     // offset 4, size 4
	width        uint32     // offset 8, size 4
	height       uint32     // offset 12, size 4
	typ          uint32     // offset 16, size 4
	discrete     v4l2_fract // offset 20, size 8
	_            [24]byte   // filler
}
