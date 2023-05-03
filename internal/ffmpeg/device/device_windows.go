package device

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"os/exec"
	"regexp"
)

// https://trac.ffmpeg.org/wiki/DirectShow
const deviceInputPrefix = "-f dshow"

func deviceInputSuffix(video, audio string) string {
	switch {
	case video != "" && audio != "":
		return `video="` + video + `":audio=` + audio + `"`
	case video != "":
		return `video="` + video + `"`
	case audio != "":
		return `audio="` + audio + `"`
	}
	return ""
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

		stream := api.Stream{
			Name: name, URL: "ffmpeg:device?" + kind + "=" + name,
		}

		switch kind {
		case core.KindVideo:
			videos = append(videos, name)
			stream.URL += "#video=h264#hardware"
		case core.KindAudio:
			audios = append(audios, name)
		}

		streams = append(streams, stream)
	}
}
