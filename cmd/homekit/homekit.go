package homekit

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("homekit")

	streams.HandleFunc("homekit", streamHandler)

	api.HandleFunc("/api/homekit", apiHandler)
}

var log zerolog.Logger

func streamHandler(url string) (streamer.Producer, error) {
	client, err := homekit.NewClient(url)
	if err != nil {
		return nil, err
	}
	if err = client.Dial(); err != nil {
		return nil, err
	}

	// start gorutine for reading responses from camera
	go func() {
		if err = client.Handle(); err != nil {
			log.Warn().Err(err).Msg("[homekit] client")
		}
	}()

	return &Producer{client: client}, nil
}
