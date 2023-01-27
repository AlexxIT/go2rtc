package device

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"os/exec"
	"strings"
)

// https://trac.ffmpeg.org/wiki/DirectShow
const deviceInputPrefix = "-f dshow"

func deviceInputSuffix(videoIdx, audioIdx int) string {
	video := findMedia(streamer.KindVideo, videoIdx)
	audio := findMedia(streamer.KindAudio, audioIdx)
	switch {
	case video != nil && audio != nil:
		return `video="` + video.MID + `":audio=` + audio.MID + `"`
	case video != nil:
		return `video="` + video.MID + `"`
	case audio != nil:
		return `audio="` + audio.MID + `"`
	}
	return ""
}

func loadMedias() {
	cmd := exec.Command(
		Bin, "-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "",
	)

	var buf bytes.Buffer
	cmd.Stderr = &buf
	_ = cmd.Run()

	lines := strings.Split(buf.String(), "\r\n")
	for _, line := range lines {
		var kind string
		if strings.HasSuffix(line, "(video)") {
			kind = streamer.KindVideo
		} else if strings.HasSuffix(line, "(audio)") {
			kind = streamer.KindAudio
		} else {
			continue
		}

		// hope we have constant prefix and suffix sizes
		// [dshow @ 00000181e8d028c0] "VMware Virtual USB Video Device" (video)
		name := line[28 : len(line)-9]
		media := loadMedia(kind, name)
		medias = append(medias, media)
	}
}

func loadMedia(kind, name string) *streamer.Media {
	return &streamer.Media{Kind: kind, MID: name}
}
