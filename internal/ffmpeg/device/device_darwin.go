//go:build darwin || ios

package device

import (
	"net/url"
	"os/exec"
	"regexp"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func queryToInput(query url.Values) string {
	video := query.Get("video")
	audio := query.Get("audio")

	if video == "" && audio == "" {
		return ""
	}

	// https://ffmpeg.org/ffmpeg-devices.html#avfoundation
	input := "-f avfoundation"

	if video != "" {
		video = indexToItem(videos, video)

		for key, value := range query {
			switch key {
			case "resolution":
				input += " -video_size " + value[0]
			case "pixel_format", "framerate", "video_size", "capture_cursor", "capture_mouse_clicks", "capture_raw_data":
				input += " -" + key + " " + value[0]
			}
		}
	}

	if audio != "" {
		audio = indexToItem(audios, audio)
	}

	return input + ` -i "` + video + `:` + audio + `"`
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

		streams = append(streams, &api.Source{
			Name: name, URL: "ffmpeg:device?" + kind + "=" + name,
		})
	}
}
