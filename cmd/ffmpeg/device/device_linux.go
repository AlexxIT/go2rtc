package device

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"io/ioutil"
	"os/exec"
	"strings"
)

// https://trac.ffmpeg.org/wiki/Capture/Webcam
const deviceInputPrefix = "-f v4l2"

func deviceInputSuffix(videoIdx, audioIdx int) string {
	if video := findMedia(core.KindVideo, videoIdx); video != nil {
		return video.ID
	}
	return ""
}

func loadMedias() {
	files, err := ioutil.ReadDir("/dev")
	if err != nil {
		return
	}
	for _, file := range files {
		log.Trace().Msg("[ffmpeg] " + file.Name())
		if strings.HasPrefix(file.Name(), core.KindVideo) {
			media := loadMedia(core.KindVideo, "/dev/"+file.Name())
			if media != nil {
				medias = append(medias, media)
			}
		}
	}
}

func loadMedia(kind, name string) *core.Media {
	cmd := exec.Command(
		Bin, "-hide_banner", "-f", "v4l2", "-list_formats", "all", "-i", name,
	)
	var buf bytes.Buffer
	cmd.Stderr = &buf
	_ = cmd.Run()

	if !bytes.Contains(buf.Bytes(), []byte("Raw")) {
		return nil
	}

	return &core.Media{Kind: kind, ID: name}
}
