//go:build windows

package device

import (
	"net/url"
	"os/exec"
	"regexp"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func queryToInput(query url.Values) string {
	video := query.Get("video")
	audio := query.Get("audio")

	if video == "" && audio == "" {
		return ""
	}

	// https://ffmpeg.org/ffmpeg-devices.html#dshow
	input := "-f dshow"

	if video != "" {
		video = indexToItem(videos, video)

		for key, value := range query {
			switch key {
			case "resolution":
				input += " -video_size " + value[0]
			case "video_size", "framerate", "pixel_format":
				input += " -" + key + " " + value[0]
			}
		}
	}

	if audio != "" {
		audio = indexToItem(audios, audio)

		for key, value := range query {
			switch key {
			case "sample_rate", "sample_size", "channels", "audio_buffer_size":
				input += " -" + key + " " + value[0]
			}
		}
	}

	if video != "" {
		input += ` -i "video=` + video

		if audio != "" {
			input += `:audio=` + audio
		}

		input += `"`
	} else {
		input += ` -i "audio=` + audio + `"`
	}

	return input
}

func initDevices() {
	cmd := exec.Command(
		Bin, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "",
	)
	b, _ := cmd.CombinedOutput()

	re := regexp.MustCompile(`"([^"]+)" \((video|audio)\)`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		name := m[1]
		kind := m[2]

		stream := &api.Source{
			Name: name, URL: "ffmpeg:device?" + kind + "=" + name,
		}

		switch kind {
		case core.KindVideo:
			videos = append(videos, name)
			stream.URL += "#video=h264#hardware"
		case core.KindAudio:
			audios = append(audios, name)
			stream.URL += "&channels=1&sample_rate=16000&audio_buffer_size=10"
		}

		streams = append(streams, stream)
	}
}
