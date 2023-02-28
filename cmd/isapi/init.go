package isapi

import (
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/isapi"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func Init() {
	streams.HandleFunc("isapi", handle)
}

func handle(url string) (streamer.Producer, error) {
	conn, err := isapi.NewClient(url)
	if err != nil {
		return nil, err
	}
	if err = conn.Dial(); err != nil {
		return nil, err
	}
	return conn, nil
}
