package ffmpeg

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

var checkMu sync.Mutex
var checkErr error
var checkVer string

const (
	FFmpeg50 = "59. 16"
	FFmpeg51 = "59. 27"
	FFmpeg60 = "60.  3"
	FFmpeg61 = "60. 16"
	FFmpeg70 = "61.  1"
)

func Version() (string, error) {
	checkMu.Lock()
	defer checkMu.Unlock()

	if checkVer != "" {
		return checkVer, checkErr
	}

	cmd := exec.Command(defaults["bin"], "-version")
	b, err := cmd.Output()
	if err != nil {
		checkVer = "-"
		checkErr = err
		return checkVer, checkErr
	}

	if len(b) < 100 {
		checkVer = "?"
		return checkVer, nil
	}

	// ffmpeg version n7.0-30-g8b0fe91754-20240520 Copyright (c) 2000-2024 the FFmpeg developers
	b = b[15:]
	if i := bytes.IndexByte(b, ' '); i > 0 {
		checkVer = string(b[:i])
	}

	// libavformat    60. 16.100 / 60. 16.100
	if i := strings.Index(string(b), "libavformat"); i > 0 {
		// better to compare libavformat, because nightly/master builds
		libav := string(b[i+15 : i+25])
		if libav < FFmpeg50 {
			checkErr = errors.New("ffmpeg: unsupported version: " + checkVer)
			return checkVer, checkErr
		}
		if libav < FFmpeg61 && strings.Contains(defaults["file"], "readrate_initial_burst") {
			defaults["file"] = "-re -i {input}"
		}
	}

	log.Debug().Str("version", checkVer).Msgf("[ffmpeg] bin")

	return checkVer, nil
}
