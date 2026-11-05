//go:build windows

package baichuan

import (
	"syscall"
)

func setBroadcastOption(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}
