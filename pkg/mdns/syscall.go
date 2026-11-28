//go:build !(darwin || ios || freebsd || openbsd || netbsd || dragonfly || windows)

package mdns

import (
	"syscall"
)

// Linux IPv6 socket-option numbers from <netinet/in.h>.
// On other Unix-like targets covered by this file (rare — no Solaris/AIX
// support is documented in the project) the values may differ; build with
// the proper per-OS file when adding such a target.
const (
	ipv6MulticastLoop = 19 // IPV6_MULTICAST_LOOP
	ipv6JoinGroup     = 20 // IPV6_JOIN_GROUP
)

func SetsockoptInt(fd uintptr, level, opt int, value int) (err error) {
	return syscall.SetsockoptInt(int(fd), level, opt, value)
}

func SetsockoptIPMreq(fd uintptr, level, opt int, mreq *syscall.IPMreq) (err error) {
	return syscall.SetsockoptIPMreq(int(fd), level, opt, mreq)
}

func SetsockoptIPv6Mreq(fd uintptr, level, opt int, mreq *syscall.IPv6Mreq) (err error) {
	return syscall.SetsockoptIPv6Mreq(int(fd), level, opt, mreq)
}
