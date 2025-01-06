//go:build !(linux && (386 || arm || amd64 || arm64))

package v4l2

func Init() {
	// not supported
}
