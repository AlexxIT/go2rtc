package homekit

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/srtp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("homekit")

	streams.HandleFunc("homekit", streamHandler)

	api.HandleFunc("api/homekit", apiHandler)
}

var log zerolog.Logger

func streamHandler(url string) (streamer.Producer, error) {
	conn, err := homekit.NewClient(url, srtp.Server)
	if err != nil {
		return nil, err
	}
	if err = conn.Dial();err!=nil{
		return nil, err
	}
	return conn, nil
}
