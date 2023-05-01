package dvrip

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/dvrip"
)

func Init() {
	streams.HandleFunc("dvrip", handle)
}

func handle(url string) (core.Producer, error) {
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
