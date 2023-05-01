package device

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"os/exec"
	"strings"
)

// https://trac.ffmpeg.org/wiki/DirectShow
const deviceInputPrefix = "-f dshow"

func deviceInputSuffix(videoIdx, audioIdx int) string {
	video := findMedia(core.KindVideo, videoIdx)
	audio := findMedia(core.KindAudio, audioIdx)
	switch {
	case video != nil && audio != nil:
		return `video="` + video.ID + `":audio=` + audio.ID + `"`
	case video != nil:
		return `video="` + video.ID + `"`
	case audio != nil:
		return `audio="` + audio.ID + `"`
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
			kind = core.KindVideo
		} else if strings.HasSuffix(line, "(audio)") {
			kind = core.KindAudio
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

func loadMedia(kind, name string) *core.Media {
	return &core.Media{Kind: kind, ID: name}
}
