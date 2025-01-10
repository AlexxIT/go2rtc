//go:build linux

package device

import (
	"bytes"
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

type Device struct {
	fd   int
	bufs [][]byte
}

func Open(path string) (*Device, error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return &Device{fd: fd}, nil
}

const buffersCount = 2

type Capability struct {
	Driver  string
	Card    string
	BusInfo string
	Version string
}

func (d *Device) Capability() (*Capability, error) {
	c := v4l2_capability{}
	if err := ioctl(d.fd, VIDIOC_QUERYCAP, unsafe.Pointer(&c)); err != nil {
		return nil, err
	}
	return &Capability{
		Driver:  str(c.driver[:]),
		Card:    str(c.card[:]),
		BusInfo: str(c.bus_info[:]),
		Version: fmt.Sprintf("%d.%d.%d", byte(c.version>>16), byte(c.version>>8), byte(c.version)),
	}, nil
}

func (d *Device) ListFormats() ([]uint32, error) {
	var items []uint32

	for i := uint32(0); ; i++ {
		fd := v4l2_fmtdesc{
			index: i,
			typ:   V4L2_BUF_TYPE_VIDEO_CAPTURE,
		}
		if err := ioctl(d.fd, VIDIOC_ENUM_FMT, unsafe.Pointer(&fd)); err != nil {
			if !errors.Is(err, syscall.EINVAL) {
				return nil, err
			}
			break
		}

		items = append(items, fd.pixelformat)
	}

	return items, nil
}

func (d *Device) ListSizes(pixFmt uint32) ([][2]uint32, error) {
	var items [][2]uint32

	for i := uint32(0); ; i++ {
		fs := v4l2_frmsizeenum{
			index:        i,
			pixel_format: pixFmt,
		}
		if err := ioctl(d.fd, VIDIOC_ENUM_FRAMESIZES, unsafe.Pointer(&fs)); err != nil {
			if !errors.Is(err, syscall.EINVAL) {
				return nil, err
			}
			break
		}

		if fs.typ != V4L2_FRMSIZE_TYPE_DISCRETE {
			continue
		}

		items = append(items, [2]uint32{fs.discrete.width, fs.discrete.height})
	}

	return items, nil
}

func (d *Device) ListFrameRates(pixFmt, width, height uint32) ([]uint32, error) {
	var items []uint32

	for i := uint32(0); ; i++ {
		fi := v4l2_frmivalenum{
			index:        i,
			pixel_format: pixFmt,
			width:        width,
			height:       height,
		}
		if err := ioctl(d.fd, VIDIOC_ENUM_FRAMEINTERVALS, unsafe.Pointer(&fi)); err != nil {
			if !errors.Is(err, syscall.EINVAL) {
				return nil, err
			}
			break
		}

		if fi.typ != V4L2_FRMIVAL_TYPE_DISCRETE || fi.discrete.numerator != 1 {
			continue
		}

		items = append(items, fi.discrete.denominator)
	}

	return items, nil
}

func (d *Device) SetFormat(width, height, pixFmt uint32) error {
	f := v4l2_format{
		typ: V4L2_BUF_TYPE_VIDEO_CAPTURE,
		pix: v4l2_pix_format{
			width:       width,
			height:      height,
			pixelformat: pixFmt,
			field:       V4L2_FIELD_NONE,
			colorspace:  V4L2_COLORSPACE_DEFAULT,
		},
	}
	return ioctl(d.fd, VIDIOC_S_FMT, unsafe.Pointer(&f))
}

func (d *Device) SetParam(fps uint32) error {
	p := v4l2_streamparm{
		typ: V4L2_BUF_TYPE_VIDEO_CAPTURE,
		capture: v4l2_captureparm{
			timeperframe: v4l2_fract{numerator: 1, denominator: fps},
		},
	}
	return ioctl(d.fd, VIDIOC_S_PARM, unsafe.Pointer(&p))
}

func (d *Device) StreamOn() (err error) {
	rb := v4l2_requestbuffers{
		count:  buffersCount,
		typ:    V4L2_BUF_TYPE_VIDEO_CAPTURE,
		memory: V4L2_MEMORY_MMAP,
	}
	if err = ioctl(d.fd, VIDIOC_REQBUFS, unsafe.Pointer(&rb)); err != nil {
		return err
	}

	d.bufs = make([][]byte, buffersCount)
	for i := uint32(0); i < buffersCount; i++ {
		qb := v4l2_buffer{
			index:  i,
			typ:    V4L2_BUF_TYPE_VIDEO_CAPTURE,
			memory: V4L2_MEMORY_MMAP,
		}
		if err = ioctl(d.fd, VIDIOC_QUERYBUF, unsafe.Pointer(&qb)); err != nil {
			return err
		}

		if d.bufs[i], err = syscall.Mmap(
			d.fd, int64(qb.offset), int(qb.length), syscall.PROT_READ, syscall.MAP_SHARED,
		); nil != err {
			return err
		}

		if err = ioctl(d.fd, VIDIOC_QBUF, unsafe.Pointer(&qb)); err != nil {
			return err
		}
	}

	typ := uint32(V4L2_BUF_TYPE_VIDEO_CAPTURE)
	return ioctl(d.fd, VIDIOC_STREAMON, unsafe.Pointer(&typ))
}

func (d *Device) StreamOff() (err error) {
	typ := uint32(V4L2_BUF_TYPE_VIDEO_CAPTURE)
	if err = ioctl(d.fd, VIDIOC_STREAMOFF, unsafe.Pointer(&typ)); err != nil {
		return err
	}

	for i := range d.bufs {
		_ = syscall.Munmap(d.bufs[i])
	}

	rb := v4l2_requestbuffers{
		count:  0,
		typ:    V4L2_BUF_TYPE_VIDEO_CAPTURE,
		memory: V4L2_MEMORY_MMAP,
	}
	return ioctl(d.fd, VIDIOC_REQBUFS, unsafe.Pointer(&rb))
}

func (d *Device) Capture(planarYUV bool) ([]byte, error) {
	dec := v4l2_buffer{
		typ:    V4L2_BUF_TYPE_VIDEO_CAPTURE,
		memory: V4L2_MEMORY_MMAP,
	}
	if err := ioctl(d.fd, VIDIOC_DQBUF, unsafe.Pointer(&dec)); err != nil {
		return nil, err
	}

	buf := make([]byte, dec.bytesused)
	if planarYUV {
		YUYV2YUV(buf, d.bufs[dec.index][:dec.bytesused])
	} else {
		copy(buf, d.bufs[dec.index][:dec.bytesused])
	}

	enc := v4l2_buffer{
		typ:    V4L2_BUF_TYPE_VIDEO_CAPTURE,
		memory: V4L2_MEMORY_MMAP,
		index:  dec.index,
	}
	if err := ioctl(d.fd, VIDIOC_QBUF, unsafe.Pointer(&enc)); err != nil {
		return nil, err
	}

	return buf, nil
}

func (d *Device) Close() error {
	return syscall.Close(d.fd)
}

func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if err != 0 {
		return err
	}
	return nil
}

func str(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
