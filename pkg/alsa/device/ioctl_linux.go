package device

import (
	"bytes"
	"reflect"
	"syscall"
)

func ioctl(fd, req uintptr, arg any) error {
	var ptr uintptr
	if arg != nil {
		ptr = reflect.ValueOf(arg).Pointer()
	}
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, req, ptr)
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
