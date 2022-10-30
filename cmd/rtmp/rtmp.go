package rtmp

import (
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/rtmp"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func Init() {
	streams.HandleFunc("rtmp", handle)
	// RTMPT (flv over HTTP)
	streams.HandleFunc("http", handle)
	streams.HandleFunc("https", handle)
}

func handle(url string) (streamer.Producer, error) {
	conn := rtmp.NewClient(url)
	if err := conn.Dial(); err != nil {
		return nil, err
	}
	return conn, nil
}
