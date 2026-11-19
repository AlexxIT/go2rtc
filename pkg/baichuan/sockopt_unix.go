//go:build !windows

package baichuan

import (
	"syscall"
)

func setBroadcastOption(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}
