//go:build !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package macwatch

// systemNeighbors is not implemented for this platform, the module will use
// the system tool instead.
func systemNeighbors() ([]Neighbor, error) {
	return nil, nil
}
