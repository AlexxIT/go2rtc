package ngrok

import (
	"fmt"
	"net"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/pkg/ngrok"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Cmd string `yaml:"command"`
		} `yaml:"ngrok"`
	}

	app.LoadConfig(&cfg)

	if cfg.Mod.Cmd == "" {
		return
	}

	log = app.GetLogger("ngrok")

	ngr, err := ngrok.NewNgrok(cfg.Mod.Cmd)
	if err != nil {
		log.Error().Err(err).Msg("[ngrok] start")
	}

	ngr.Listen(func(msg any) {
		if msg := msg.(*ngrok.Message); msg != nil {
			if strings.HasPrefix(msg.Line, "ERROR:") {
				log.Warn().Msg("[ngrok] " + msg.Line)
			} else {
				log.Debug().Msg("[ngrok] " + msg.Line)
			}

			// Addr: "//localhost:8555", URL: "tcp://1.tcp.eu.ngrok.io:12345"
			if strings.HasPrefix(msg.Addr, "//localhost:") && strings.HasPrefix(msg.URL, "tcp://") {
				// don't know if really necessary use IP
				address, err := ConvertHostToIP(msg.URL[6:])
				if err != nil {
					log.Warn().Err(err).Msg("[ngrok] add candidate")
					return
				}

				log.Info().Str("addr", address).Msg("[ngrok] add external candidate for WebRTC")

				webrtc.AddCandidate("tcp", address)
			}
		}
	})

	go func() {
		if err = ngr.Serve(); err != nil {
			log.Error().Err(err).Msg("[ngrok] run")
		}
	}()

}

var log zerolog.Logger

func ConvertHostToIP(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}

	ip, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}

	if len(ip) == 0 {
		return "", fmt.Errorf("can't resolve: %s", host)
	}

	return ip[0].String() + ":" + port, nil
}
