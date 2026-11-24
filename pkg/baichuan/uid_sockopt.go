//go:build !windows

package baichuan

import (
	"errors"
	"syscall"
)

func setUIDBroadcast(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}

func isUIDMessageSize(err error) bool {
	return errors.Is(err, syscall.EMSGSIZE)
}
