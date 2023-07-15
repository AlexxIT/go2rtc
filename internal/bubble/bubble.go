package bubble

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/bubble"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func Init() {
	streams.HandleFunc("bubble", handle)
}

func handle(url string) (core.Producer, error) {
	conn := bubble.NewClient(url)
	if err := conn.Dial(); err != nil {
		return nil, err
	}
	return conn, nil
}
