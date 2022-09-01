package srtp

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/rs/zerolog"
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

	log = app.GetLogger("srtp")

	// create SRTP server (endpoint) for receiving video from HomeKit camera
	conn, err := net.ListenPacket("udp", cfg.Mod.Listen)
	if err != nil {
		log.Warn().Err(err).Msg("[srtp] listen")
	}

	log.Info().Str("addr", cfg.Mod.Listen).Msg("[srtp] listen")

	_, Port, _ = net.SplitHostPort(cfg.Mod.Listen)

	// run server
	go func() {
		server = &srtp.Server{}
		if err = server.Serve(conn); err != nil {
			log.Warn().Err(err).Msg("[srtp] serve")
		}
	}()
}

var log zerolog.Logger
var server *srtp.Server

var Port string

func AddSession(session *srtp.Session)  {
	server.AddSession(session)
}

func RemoveSession(session *srtp.Session) {
	server.RemoveSession(session)
}