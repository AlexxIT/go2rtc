package homekit

import (
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/srtp"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("homekit")

	streams.HandleFunc("homekit", streamHandler)

	api.HandleFunc("api/homekit", apiHandler)
}

var log zerolog.Logger

func streamHandler(url string) (core.Producer, error) {
	return homekit.Dial(url, srtp.Server)
}

func findHomeKitURL(stream *streams.Stream) string {
	sources := stream.Sources()
	if len(sources) == 0 {
		return ""
	}

	url := sources[0]
	if strings.HasPrefix(url, "homekit") {
		return url
	}

	if strings.HasPrefix(url, "hass") {
		location, _ := streams.Location(url)
		if strings.HasPrefix(location, "homekit") {
			return url
		}
	}

	return ""
}
