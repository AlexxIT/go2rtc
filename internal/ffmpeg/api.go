package ffmpeg

import (
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
)

func apiFFmpeg(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query()
	dst := query.Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	var src string
	if s := query.Get("file"); s != "" {
		if streams.Validate(s) == nil {
			src = "ffmpeg:" + s + "#audio=auto#input=file"
		}
	} else if s = query.Get("live"); s != "" {
		if streams.Validate(s) == nil {
			src = "ffmpeg:" + s + "#audio=auto"
		}
	} else if s = query.Get("text"); s != "" {
		if strings.IndexAny(s, `'"&%$`) < 0 {
			src = "ffmpeg:tts?text=" + s
			if s = query.Get("voice"); s != "" {
				src += "&voice=" + s
			}
			src += "#audio=auto"
		}
	}

	if src == "" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	if err := stream.Play(src); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
