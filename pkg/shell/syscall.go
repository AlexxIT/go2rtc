//go:build linux || (aix && ppc) || (aix && ppc64) || (darwin && amd64) || (darwin && arm64) || (dragonfly && amd64) || (freebsd && 386) || (freebsd && amd64) || (freebsd && arm) || (freebsd && arm64) || (freebsd && riscv64) || (netbsd && 386) || (netbsd && amd64) || (netbsd && arm) || (netbsd && arm64) || (openbsd && 386) || (openbsd && amd64) || (openbsd && arm) || (openbsd && arm64) || (openbsd && mips64) || (openbsd && ppc64) || (openbsd && riscv64) || (solaris && amd64) || (zos && s390x)
// +build linux aix,ppc aix,ppc64 darwin,amd64 darwin,arm64 dragonfly,amd64 freebsd,386 freebsd,amd64 freebsd,arm freebsd,arm64 freebsd,riscv64 netbsd,386 netbsd,amd64 netbsd,arm netbsd,arm64 openbsd,386 openbsd,amd64 openbsd,arm openbsd,arm64 openbsd,mips64 openbsd,ppc64 openbsd,riscv64 solaris,amd64 zos,s390x

// Generated with: for line in `grep '^func Setgid' vendor/golang.org/x/sys/unix/*syscall_*.go | cut -d':' -f1`; do os=$(basename "${line%.go}" | awk -F_ '{print $2}'); arch=$(basename "$line" | awk -F_ '{print $3}' | awk -F. '{print $1}'); osarch="$os,$arch"; echo -n "${osarch%,} "; done

package shell

import (
	"github.com/rs/zerolog/log"
	"os"
	"syscall"
)

func CheckRootAndDropPrivileges() {
	if os.Getuid() == 0 {
		// Drop privileges
		err := syscall.Setgid(int(GetForkGroupId()))
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to setgid: %v", err)
		}
		err = syscall.Setuid(int(GetForkUserId()))
		if err != nil {
			log.Fatal().Err(err).Msgf("Failed to setuid: %v", err)
		}
	} else {
		log.Fatal().Msgf("You must run go2rtc as root in order to use the 'user' flag")
		os.Exit(1)
	}
}
