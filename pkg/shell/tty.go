//go:build !unix

package shell

func IsInteractive(fd uintptr) bool {
	return false
}
