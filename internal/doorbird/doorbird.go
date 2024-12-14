package doorbird

import (
	"net/url"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/doorbird"
)

func Init() {
	streams.RedirectFunc("doorbird", func(rawURL string) (string, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return "", err
		}

		// https://www.doorbird.com/downloads/api_lan.pdf
		switch u.Query().Get("media") {
		case "video":
			u.Path = "/bha-api/video.cgi"
		case "audio":
			u.Path = "/bha-api/audio-receive.cgi"
		default:
			return "", nil
		}

		u.Scheme = "http"

		return u.String(), nil
	})

	streams.HandleFunc("doorbird", func(source string) (core.Producer, error) {
		return doorbird.Dial(source)
	})
}
