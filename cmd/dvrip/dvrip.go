package dvrip

import (
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/dvrip"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func Init() {
	streams.HandleFunc("dvrip", handle)
}

func handle(url string) (streamer.Producer, error) {
	conn := dvrip.NewClient(url)
	if err := conn.Dial(); err != nil {
		return nil, err
	}
	if err := conn.Play(); err != nil {
		return nil, err
	}
	if err := conn.Handle(); err != nil {
		return nil, err
	}
	return conn, nil
}
