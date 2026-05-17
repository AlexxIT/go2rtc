package reolink

import (
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/reolink"
)

func Init() {
	streams.HandleFunc("reolink", func(source string) (core.Producer, error) {
		client, err := reolink.Dial(source)
		if err != nil {
			return nil, err
		}
		if err := client.Probe(); err != nil {
			client.Close()
			return nil, err
		}
		return client, nil
	})
}
