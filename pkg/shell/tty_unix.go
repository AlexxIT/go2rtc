//go:build unix

package shell

import "golang.org/x/sys/unix"

func IsInteractive(fd uintptr) bool {
	_, err := unix.IoctlGetTermios(int(fd), unix.TIOCGETA)
	return err == nil
}
