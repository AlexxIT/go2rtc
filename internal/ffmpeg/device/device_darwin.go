package device

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"os/exec"
	"regexp"
	"strings"
)

// https://trac.ffmpeg.org/wiki/Capture/Webcam
const deviceInputPrefix = "-f avfoundation"

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
}
