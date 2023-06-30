package device

import (
	"bytes"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"syscall"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

// https://trac.ffmpeg.org/wiki/Capture/Webcam
var deviceInputPrefix = "-f avfoundation"

func deviceInputSuffix(video, audio string) string {
	switch {
	case video != "" && audio != "":
		return `"` + video + `:` + audio + `"`
	case video != "":
		return `"` + video + `"`
	case audio != "":
		return `":` + audio + `"`
	}
	return ""
}

func initDevices() {
	// [AVFoundation indev @ 0x147f04510] AVFoundation video devices:
	// [AVFoundation indev @ 0x147f04510] [0] FaceTime HD Camera
	// [AVFoundation indev @ 0x147f04510] [1] Capture screen 0
	// [AVFoundation indev @ 0x147f04510] AVFoundation audio devices:
	// [AVFoundation indev @ 0x147f04510] [0] MacBook Pro Microphone
	cmd := exec.Command(
		Bin, "-hide_banner", "-list_devices", "true", "-f", "avfoundation", "-i", "",
	)
	b, _ := cmd.CombinedOutput()

	re := regexp.MustCompile(`\[\d+] (.+)`)

	var kind string
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasSuffix(line, "video devices:"):
			kind = core.KindVideo
			continue
		case strings.HasSuffix(line, "audio devices:"):
			kind = core.KindAudio
			continue
		}

		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		name := m[1]

		switch kind {
		case core.KindVideo:
			videos = append(videos, name)
		case core.KindAudio:
			audios = append(audios, name)
		}

		streams = append(streams, api.Stream{
			Name: name, URL: "ffmpeg:device?" + kind + "=" + name,
		})
	}

	inputFormats, err := detectInputFormats("0")
	if err == nil {
		deviceInputPrefix += " -pix_fmt:v " + inputFormats[0]
	}
}

func detectInputFormats(video string) ([]string, error) {

	cmd := exec.Command("ffprobe", "-hide_banner", "-v", "error", "-print_format", "json", "-f", "avfoundation", "-video_size", "640x480", "-framerate", "24", "-i", video)
	log.Debug().Msgf("[device_darwin] %s", cmd.String())
	var out bytes.Buffer
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		// If it's an exit error, get the exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			log.Warn().Msgf("Exit code: %d\n", ws.ExitStatus())
		} else {
			// Not an ExitError, just print the error
			log.Warn().Msgf("Command finished with error: %v\n", err)
		}

		return nil, err
	}

	lines := strings.Split(out.String(), "\n")

	var results []string
	for _, value := range lines {
		parts := strings.Fields(value)
		if len(parts) != 4 {
			continue
		} else {
			results = append(results, parts[3])
		}
	}

	if len(results) == 0 {
		return nil, errors.New("empty formats")
	}
	return results, nil
}
