//go:build !(linux && (386 || amd64 || arm || arm64 || mipsle))

package alsa

func Init() {
	// not supported
}
