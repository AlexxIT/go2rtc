package srtp

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"net"
)

var cfg struct {
	Mod struct {
		Listen string `yaml:"listen"`
	} `yaml:"srtp"`
}

func Init() {

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

// ReloadConfig reloads the srtp configuration and restarts the server
func ReloadConfig() {

	// default config
	cfg.Mod.Listen = ":8443"

	// load config from YAML
	app.LoadConfig(&cfg)

	if cfg.Mod.Listen == "" {
		return
	}

	// Stop the old server and close the connection
	if Server != nil {
		Server.Close()
	}

	// Start a new server with the updated configuration
	log := app.GetLogger("srtp")

	conn, err := net.ListenPacket("udp", cfg.Mod.Listen)
	if err != nil {
		log.Warn().Err(err).Caller().Send()
	}

	log.Info().Str("addr", cfg.Mod.Listen).Msg("[srtp] listen")

	go func() {
		Server = &srtp.Server{}
		if err = Server.Serve(conn); err != nil {
			log.Warn().Err(err).Caller().Send()
		}
	}()
}

var Server *srtp.Server
