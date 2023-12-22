//go:build !(linux || (aix && ppc) || (aix && ppc64) || (darwin && amd64) || (darwin && arm64) || (dragonfly && amd64) || (freebsd && 386) || (freebsd && amd64) || (freebsd && arm) || (freebsd && arm64) || (freebsd && riscv64) || (netbsd && 386) || (netbsd && amd64) || (netbsd && arm) || (netbsd && arm64) || (openbsd && 386) || (openbsd && amd64) || (openbsd && arm) || (openbsd && arm64) || (openbsd && mips64) || (openbsd && ppc64) || (openbsd && riscv64) || (solaris && amd64) || (zos && s390x))

package shell

import (
	"github.com/rs/zerolog/log"
	"os"
	"runtime"
)

func CheckRootAndDropPrivileges() {
	// There's no equivalent of Unix's Setuid/Setgid in Windows, you might want to handle this case differently.
	log.Fatal().Msgf("You cannot use the 'user' flag in %s/%s", runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
