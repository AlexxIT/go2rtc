package pinggy

import (
	"net/url"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/pinggy"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Tunnel string `yaml:"tunnel"`
		} `yaml:"pinggy"`
	}

	app.LoadConfig(&cfg)

	if cfg.Mod.Tunnel == "" {
		return
	}

	log = app.GetLogger("pinggy")

	u, err := url.Parse(cfg.Mod.Tunnel)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	go proxy(u.Scheme, u.Host)
}

var log zerolog.Logger

func proxy(proto, address string) {
	client, err := pinggy.NewClient(proto)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
	defer client.Close()

	urls, err := client.GetURLs()
	if err != nil {
		log.Error().Err(err).Send()
		return
	}

	for _, s := range urls {
		log.Info().Str("url", s).Msgf("[pinggy] proxy")
	}

	err = client.Proxy(address)
	if err != nil {
		log.Error().Err(err).Send()
		return
	}
}
