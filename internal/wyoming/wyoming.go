package wyoming

import (
	"net"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/wyoming"
	"github.com/rs/zerolog"
)

func Init() {
	streams.HandleFunc("wyoming", wyoming.Dial)

	// server
	var cfg struct {
		Mod map[string]struct {
			Listen       string  `yaml:"listen"`
			Name         string  `yaml:"name"`
			WakeURI      string  `yaml:"wake_uri"`
			VADThreshold float32 `yaml:"vad_threshold"`
		} `yaml:"wyoming"`
	}
	app.LoadConfig(&cfg)

	log = app.GetLogger("wyoming")

	for name, cfg := range cfg.Mod {
		stream := streams.Get(name)
		if stream == nil {
			log.Warn().Msgf("[wyoming] missing stream: %s", name)
			continue
		}

		ln, err := net.Listen("tcp", cfg.Listen)
		if err != nil {
			log.Warn().Msgf("[wyoming] listen error: %s", err)
			continue
		}

		if cfg.Name == "" {
			cfg.Name = name
		}

		srv := wyoming.Server{
			Name:         cfg.Name,
			VADThreshold: int16(1000 * cfg.VADThreshold), // 1.0 => 1000
			WakeURI:      cfg.WakeURI,
			MicHandler: func(cons core.Consumer) error {
				if err := stream.AddConsumer(cons); err != nil {
					return err
				}
				// not best solution
				if i, ok := cons.(interface{ OnClose(func()) }); ok {
					i.OnClose(func() {
						stream.RemoveConsumer(cons)
					})
				}
				return nil
			},
			SndHandler: func(prod core.Producer) error {
				return stream.Play(prod)
			},
		}
		go srv.Serve(ln)
	}
}

var log zerolog.Logger
