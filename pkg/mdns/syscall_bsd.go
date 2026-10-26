//go:build darwin || ios || freebsd || openbsd || netbsd || dragonfly

package mdns

import (
	"syscall"
)

// BSD constants from <netinet6/in6.h>: IPV6_MULTICAST_LOOP=11, IPV6_JOIN_GROUP=12.
const (
	ipv6MulticastLoop = 11
	ipv6JoinGroup     = 12
)

func SetsockoptInt(fd uintptr, level, opt int, value int) (err error) {
	// change SO_REUSEADDR and REUSEPORT flags simultaneously for BSD-like OS
	// https://github.com/AlexxIT/go2rtc/issues/626
	// https://stackoverflow.com/questions/14388706/how-do-so-reuseaddr-and-so-reuseport-differ/14388707
	if opt == syscall.SO_REUSEADDR {
		if err = syscall.SetsockoptInt(int(fd), level, opt, value); err != nil {
			return
		}

		opt = syscall.SO_REUSEPORT
	}

	return syscall.SetsockoptInt(int(fd), level, opt, value)
}

func SetsockoptIPMreq(fd uintptr, level, opt int, mreq *syscall.IPMreq) (err error) {
	return syscall.SetsockoptIPMreq(int(fd), level, opt, mreq)
}

func SetsockoptIPv6Mreq(fd uintptr, level, opt int, mreq *syscall.IPv6Mreq) (err error) {
	return syscall.SetsockoptIPv6Mreq(int(fd), level, opt, mreq)
}
