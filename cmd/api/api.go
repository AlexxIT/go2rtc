package api

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"net"
	"net/http"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen    string `yaml:"listen"`
			BasePath  string `yaml:"base_path"`
			StaticDir string `yaml:"static_dir"`
		} `yaml:"api"`
	}

	// default config
	cfg.Mod.Listen = ":3000"
	cfg.Mod.StaticDir = "www"

	// load config from YAML
	app.LoadConfig(&cfg)

	if cfg.Mod.Listen == "" {
		return
	}

	basePath = cfg.Mod.BasePath
	log = app.GetLogger("api")

	if cfg.Mod.StaticDir != "" {
		fileServer = http.FileServer(http.Dir(cfg.Mod.StaticDir))
		HandleFunc("/", fileServerHandlder)
	}

	HandleFunc("/api/frame.mp4", frameHandler)
	HandleFunc("/api/frame.raw", frameHandler)
	HandleFunc("/api/stack", stackHandler)
	HandleFunc("/api/stats", statsHandler)
	HandleFunc("/api/ws", apiWS)

	// ensure we can listen without errors
	listener, err := net.Listen("tcp", cfg.Mod.Listen)
	if err != nil {
		log.Fatal().Err(err).Msg("[api] listen")
	}

	log.Info().Str("addr", cfg.Mod.Listen).Msg("[api] listen")

	go func() {
		s := http.Server{}
		if err = s.Serve(listener); err != nil {
			log.Fatal().Err(err).Msg("[api] Serve")
		}
	}()
}

func HandleFunc(pattern string, handler http.HandlerFunc) {
	http.HandleFunc(basePath+pattern, handler)
}

func HandleWS(msgType string, handler WSHandler) {
	wsHandlers[msgType] = handler
}

var basePath string
var fileServer http.Handler
var log zerolog.Logger
var wsHandlers = make(map[string]WSHandler)

func fileServerHandlder(w http.ResponseWriter, r *http.Request) {
	if basePath != "" {
		r.URL.Path = r.URL.Path[len(basePath):]
	}
	fileServer.ServeHTTP(w, r)
}

func statsHandler(w http.ResponseWriter, _ *http.Request) {
	v := map[string]interface{}{
		"streams": streams.Streams,
	}
	data, err := json.Marshal(v)
	if err != nil {
		log.Error().Err(err).Msg("[api.stats] marshal")
	}
	if _, err = w.Write(data); err != nil {
		log.Error().Err(err).Msg("[api.stats] write")
	}
}

func apiWS(w http.ResponseWriter, r *http.Request) {
	ctx := new(Context)
	if err := ctx.Upgrade(w, r); err != nil {
		log.Error().Err(err).Msg("[api.ws] upgrade")
		return
	}
	defer ctx.Close()

	for {
		msg := new(streamer.Message)
		if err := ctx.Conn.ReadJSON(msg); err != nil {
			if websocket.IsUnexpectedCloseError(
				err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure,
			) {
				log.Error().Err(err).Msg("[api.ws] readJSON")
			}
			return
		}

		handler := wsHandlers[msg.Type]
		if handler != nil {
			handler(ctx, msg)
		}
	}
}
