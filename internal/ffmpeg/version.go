package ffmpeg

import (
	"errors"
	"os/exec"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/ffmpeg"
)

var verMu sync.Mutex
var verErr error
var verFF string
var verAV string

func Version() (string, error) {
	verMu.Lock()
	defer verMu.Unlock()

	if verFF != "" {
		return verFF, verErr
	}

	cmd := exec.Command(defaults["bin"], "-version")
	b, err := cmd.Output()
	if err != nil {
		verFF = "-"
		verErr = err
		return verFF, verErr
	}

	verFF, verAV = ffmpeg.ParseVersion(b)

	if verFF == "" {
		verFF = "?"
	}

	// better to compare libavformat, because nightly/master builds
	if verAV != "" && verAV < ffmpeg.Version50 {
		verErr = errors.New("ffmpeg: unsupported version: " + verFF)
	}

	log.Debug().Str("version", verFF).Str("libavformat", verAV).Msgf("[ffmpeg] bin")

	return verFF, verErr
}
