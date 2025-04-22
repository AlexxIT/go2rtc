package ioctl

import (
	"bytes"
)

func Str(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

func io(mode byte, type_ byte, number byte, size uint16) uintptr {
	return uintptr(mode)<<30 | uintptr(size)<<16 | uintptr(type_)<<8 | uintptr(number)
}

func IOR(type_ byte, number byte, size uint16) uintptr {
	return io(read, type_, number, size)
}

func IOW(type_ byte, number byte, size uint16) uintptr {
	return io(write, type_, number, size)
}

func IORW(type_ byte, number byte, size uint16) uintptr {
	return io(read|write, type_, number, size)
}
