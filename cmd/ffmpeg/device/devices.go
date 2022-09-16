package device

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func Init() {
	log = app.GetLogger("exec")

	api.HandleFunc("api/devices", handle)
}

func GetInput(src string) (string, error) {
	if medias == nil {
		loadMedias()
	}

	input := deviceInputPrefix

	var videoIdx, audioIdx int
	if i := strings.IndexByte(src, '?'); i > 0 {
		query, err := url.ParseQuery(src[i+1:])
		if err != nil {
			return "", err
		}
		for key, value := range query {
			switch key {
			case "video":
				videoIdx, _ = strconv.Atoi(value[0])
			case "audio":
				audioIdx, _ = strconv.Atoi(value[0])
			case "framerate":
				input += " -framerate " + value[0]
			case "resolution":
				input += " -video_size " + value[0]
			}
		}
	}

	input += " -i " + deviceInputSuffix(videoIdx, audioIdx)

	return input, nil
}

var Bin string
var log zerolog.Logger
var medias []*streamer.Media

func findMedia(kind string, index int) *streamer.Media {
	for _, media := range medias {
		if media.Kind != kind {
			continue
		}
		if index == 0 {
			return media
		}
		index--
	}
	return nil
}

func handle(w http.ResponseWriter, r *http.Request) {
	if medias == nil {
		loadMedias()
	}

	data, err := json.Marshal(medias)
	if err != nil {
		log.Error().Err(err).Msg("[api.ffmpeg]")
		return
	}
	if _, err = w.Write(data); err != nil {
		log.Error().Err(err).Msg("[api.ffmpeg]")
	}
}
