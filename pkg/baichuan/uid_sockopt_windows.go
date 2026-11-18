//go:build windows

package baichuan

import (
	"errors"
	"syscall"
)

func setUIDBroadcast(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}

func isUIDMessageSize(err error) bool {
	const wsaMessageSize = syscall.Errno(10040)
	return errors.Is(err, wsaMessageSize)
}
