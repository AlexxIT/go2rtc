package device

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

func Init(bin string) {
	Bin = bin

	api.HandleFunc("api/ffmpeg/devices", apiDevices)
}

func GetInput(src string) (string, error) {
	runonce.Do(initDevices)

	input := deviceInputPrefix

	var video, audio string

	if i := strings.IndexByte(src, '?'); i > 0 {
		query, err := url.ParseQuery(src[i+1:])
		if err != nil {
			return "", err
		}
		for key, value := range query {
			switch key {
			case "video":
				video = value[0]
			case "audio":
				audio = value[0]
			case "resolution":
				input += " -video_size " + value[0]
			default: // "input_format", "framerate", "video_size"
				input += " -" + key + " " + value[0]
			}
		}
	}

	if video != "" {
		if i, err := strconv.Atoi(video); err == nil && i < len(videos) {
			video = videos[i]
		}
	}
	if audio != "" {
		if i, err := strconv.Atoi(audio); err == nil && i < len(audios) {
			audio = audios[i]
		}
	}

	input += " -i " + deviceInputSuffix(video, audio)

	return input, nil
}

var Bin string

var videos, audios []string
var streams []api.Stream
var runonce sync.Once

func apiDevices(w http.ResponseWriter, r *http.Request) {
	runonce.Do(initDevices)

	api.ResponseStreams(w, streams)
}
