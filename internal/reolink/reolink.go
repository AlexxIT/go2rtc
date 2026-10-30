package reolink

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/reolink"
)

func Init() {
	log := app.GetLogger("reolink")

	streams.HandleFunc("reolink", func(source string) (core.Producer, error) {
		client, err := reolink.Dial(source)
		if err != nil {
			return nil, err
		}
		client.OnError = func(err error) {
			log.Warn().Err(err).Msgf("[reolink] talk %s", client.RemoteAddr)
		}
		return client, nil
	})
}
