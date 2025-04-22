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
			Mode         string  `yaml:"mode"`
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

		if cfg.Name == "" {
			cfg.Name = name
		}

		srv := &wyoming.Server{
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
			Trace: func(format string, v ...any) {
				log.Trace().Msgf("[wyoming] "+format, v...)
			},
		}
		go serve(srv, cfg.Mode, cfg.Listen)
	}
}

var log zerolog.Logger

func serve(srv *wyoming.Server, mode, address string) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		log.Warn().Msgf("[wyoming] listen error: %s", err)
	}

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}

		go handle(srv, mode, conn)
	}
}

func handle(srv *wyoming.Server, mode string, conn net.Conn) {
	addr := conn.RemoteAddr()

	log.Trace().Msgf("[wyoming] %s connected", addr)

	var err error

	switch mode {
	case "mic":
		err = srv.HandleMic(conn)
	default:
		err = srv.Handle(conn)
	}

	if err != nil {
		log.Error().Msgf("[wyoming] %s error: %s", addr, err)
	}

	log.Trace().Msgf("[wyoming] %s disconnected", addr)
}
