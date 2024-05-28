//go:build !unix && !windows

package shell

func IsInteractive(fd uintptr) bool {
	return false
}
