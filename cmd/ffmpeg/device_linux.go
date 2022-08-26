package ffmpeg

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog/log"
	"io/ioutil"
	"strings"
)

// https://trac.ffmpeg.org/wiki/Capture/Webcam
const deviceInputPrefix = "-f v4l2"

func deviceInputSuffix(videoIdx, audioIdx int) string {
	video := findMedia(streamer.KindVideo, videoIdx)
	return video.Title
}

func loadMedias() {
	files, err := ioutil.ReadDir("/dev")
	if err != nil {
		return
	}
	for _, file := range files {
		log.Trace().Msg("[ffmpeg] " + file.Name())
		if strings.HasPrefix(file.Name(), streamer.KindVideo) {
			media := loadMedia(streamer.KindVideo, "/dev/"+file.Name())
			medias = append(medias, media)
		}
	}
}

func loadMedia(kind, name string) *streamer.Media {
	return &streamer.Media{
		Kind: kind, Title: name,
	}
}
