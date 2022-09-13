package ivideon

import (
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/ivideon"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strings"
)

func Init() {
	streams.HandleFunc("ivideon", func(url string) (streamer.Producer, error) {
		id := strings.Replace(url[8:], "/", ":", 1)
		prod := ivideon.NewClient(id)
		if err := prod.Dial(); err != nil {
			return nil, err
		}
		return prod, nil
	})
}
