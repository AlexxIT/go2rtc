package ioctl

import (
	"syscall"
	"unsafe"
)

func Ioctl(fd int, req uint, arg unsafe.Pointer) error {
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg))
	if err != 0 {
		return err
	}
	return nil
}
