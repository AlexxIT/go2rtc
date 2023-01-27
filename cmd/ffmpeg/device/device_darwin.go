package device

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"os/exec"
	"strings"
)

// https://trac.ffmpeg.org/wiki/Capture/Webcam
const deviceInputPrefix = "-f avfoundation"

func deviceInputSuffix(videoIdx, audioIdx int) string {
	video := findMedia(streamer.KindVideo, videoIdx)
	audio := findMedia(streamer.KindAudio, audioIdx)
	switch {
	case video != nil && audio != nil:
		return `"` + video.MID + `:` + audio.MID + `"`
	case video != nil:
		return `"` + video.MID + `"`
	case audio != nil:
		return `"` + audio.MID + `"`
	}
	return ""
}

func loadMedias() {
	cmd := exec.Command(
		Bin, "-hide_banner", "-list_devices", "true", "-f", "avfoundation", "-i", "dummy",
	)

	var buf bytes.Buffer
	cmd.Stderr = &buf
	_ = cmd.Run()

	var kind string

	lines := strings.Split(buf.String(), "\n")
process:
	for _, line := range lines {
		switch {
		case strings.HasSuffix(line, "video devices:"):
			kind = streamer.KindVideo
			continue
		case strings.HasSuffix(line, "audio devices:"):
			kind = streamer.KindAudio
			continue
		case strings.HasPrefix(line, "dummy"):
			break process
		}

		// [AVFoundation indev @ 0x7fad54604380] [0] FaceTime HD Camera
		name := line[42:]
		media := loadMedia(kind, name)
		medias = append(medias, media)
	}
}

func loadMedia(kind, name string) *streamer.Media {
	return &streamer.Media{Kind: kind, MID: name}
}
