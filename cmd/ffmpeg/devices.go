package ffmpeg

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog/log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func getDevice(src string) (string, error) {
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

func handleDevices(w http.ResponseWriter, r *http.Request) {
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
