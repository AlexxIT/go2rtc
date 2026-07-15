//go:build windows

package mdns

import "syscall"

// Windows IPv6 socket-option numbers (winsock2/ws2ipdef.h).
// IPV6_MULTICAST_LOOP=11, IPV6_JOIN_GROUP=12 — the same as the BSD family
// and different from Linux (19/20).
const (
	ipv6MulticastLoop = 11
	ipv6JoinGroup     = 12
)

func SetsockoptInt(fd uintptr, level, opt int, value int) (err error) {
	return syscall.SetsockoptInt(syscall.Handle(fd), level, opt, value)
}

func SetsockoptIPMreq(fd uintptr, level, opt int, mreq *syscall.IPMreq) (err error) {
	return syscall.SetsockoptIPMreq(syscall.Handle(fd), level, opt, mreq)
}

func SetsockoptIPv6Mreq(fd uintptr, level, opt int, mreq *syscall.IPv6Mreq) (err error) {
	return syscall.SetsockoptIPv6Mreq(syscall.Handle(fd), level, opt, mreq)
}
