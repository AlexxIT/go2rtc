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
	video_device_id := query.Get("video_device_id")
	audio_device_id := query.Get("audio_device_id")

	if video == "" && audio == "" && video_device_id == "" && audio_device_id == "" {
		return ""
	}

	// https://ffmpeg.org/ffmpeg-devices.html#avfoundation
	input := "-f avfoundation"

	if video != "" || video_device_id != "" {
		for key, value := range query {
			switch key {
			case "resolution":
				input += " -video_size " + value[0]
			case "pixel_format", "framerate", "video_size", "capture_cursor", "capture_mouse_clicks", "capture_raw_data":
				input += " -" + key + " " + value[0]
			}
		}

		if video_device_id != "" {
			input += " -video_device_id " + video_device_id
			video = ""
		} else {
			video = indexToItem(videos, video)
		}
	}

	if audio != "" || audio_device_id != "" {
		if audio_device_id != "" {
			input += " -audio_device_id " + audio_device_id
			audio = ""
		} else {
			audio = indexToItem(audios, audio)
		}
	}

	return input + ` -i "` + video + `:` + audio + `"`
}

func initDevices() {
	// [AVFoundation indev @ 0x1500047d0] AVFoundation video devices:
	// [AVFoundation indev @ 0x1500047d0] [0] FaceTime HD Camera  [uid:XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX]
	// [AVFoundation indev @ 0x1500047d0] [1] Capture screen 0
	// [AVFoundation indev @ 0x1500047d0] AVFoundation audio devices:
	// [AVFoundation indev @ 0x1500047d0] [0] MacBook Air Microphone  [uid:BuiltInMicrophoneDevice]
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
		// remove unique IDs from device name output
		if i := strings.Index(name, "[uid:"); i >= 0 {
			name = strings.TrimSpace(name[:i])
		}

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
