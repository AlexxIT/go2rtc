package srtp

import (
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"net"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen string `yaml:"listen"`
		} `yaml:"srtp"`
	}

	// default config
	cfg.Mod.Listen = ":8443"

	// load config from YAML
	app.LoadConfig(&cfg)

	if cfg.Mod.Listen == "" {
		return
	}

	log := app.GetLogger("srtp")

	// create SRTP server (endpoint) for receiving video from HomeKit camera
	conn, err := net.ListenPacket("udp", cfg.Mod.Listen)
	if err != nil {
		log.Warn().Err(err).Caller().Send()
	}

	log.Info().Str("addr", cfg.Mod.Listen).Msg("[srtp] listen")

	// run server
	go func() {
		Server = &srtp.Server{}
		if err = Server.Serve(conn); err != nil {
			log.Warn().Err(err).Caller().Send()
		}
	}()
}

var Server *srtp.Server
