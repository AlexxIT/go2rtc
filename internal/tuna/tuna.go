package tuna

import (
	"fmt"
	"net"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"github.com/AlexxIT/go2rtc/pkg/tuna"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Cmd string `yaml:"command"`
		} `yaml:"tuna"`
	}

	app.LoadConfig(&cfg)

	if cfg.Mod.Cmd == "" {
		return
	}

	log = app.GetLogger("tuna")

	tun, err := tuna.NewTuna(cfg.Mod.Cmd)
	if err != nil {
		log.Error().Err(err).Msg("[tuna] start")
	}

	tun.Listen(func(msg any) {
		if msg := msg.(*tuna.Message); msg != nil {
			if strings.HasPrefix(msg.Line, "Error:") {
				log.Warn().Msg("[tuna] " + msg.Line)
			} else if msg.Level == "error" {
				log.Warn().Msg("[tuna] " + msg.Line)
			} else {
				log.Debug().Msg("[tuna] " + msg.Line)
			}

			// Addr: "127.0.0.1:8555", URL: "tcp://ru.tuna.am:12345"
			if strings.HasPrefix(msg.Addr, "127.0.0.1:") && strings.HasPrefix(msg.URL, "tcp://") {
				// don't know if really necessary use IP
				address, err := ConvertHostToIP(msg.URL[6:])
				if err != nil {
					log.Warn().Err(err).Msg("[tuna] add candidate")
					return
				}
				port, err := GetPort(msg.Addr)
				if err != nil {
					log.Warn().Err(err).Msg("[tuna] get port")
					return
				}
				if port == "8555" {
					log.Info().Str("addr", address).Msg("[tuna] add external candidate for WebRTC")
					webrtc.AddCandidate("tcp", address)
				}
			}
		}
	})

	go func() {
		if err = tun.Serve(); err != nil {
			log.Error().Err(err).Msg("[tuna] run")
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

func GetPort(address string) (string, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", err
	}
	return port, nil
}
