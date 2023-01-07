package api

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"
)

type apiCfg struct {
	Mod struct {
		Listen    string `yaml:"listen"`
		BasePath  string `yaml:"base_path"`
		StaticDir string `yaml:"static_dir"`
		Origin    string `yaml:"origin"`
	} `yaml:"api"`
	RTSP struct {
		Listen string `yaml:"listen"`
	} `yaml:"rtsp"`
}

func Init() {

	var cfg apiCfg

	// default config
	cfg.Mod.Listen = ":1984"
	cfg.RTSP.Listen = ":8554"

	// load config from YAML
	app.LoadConfig(&cfg)

	if cfg.Mod.Listen == "" {
		return
	}

	basePath = cfg.Mod.BasePath
	log = app.GetLogger("api")

	initStatic(cfg)
	initWS(cfg.Mod.Origin)

	HandleFunc("api/streams", streamsHandler)
	HandleFunc("api/ws", apiWS)
	HandleFunc("api/saveConfig", handleSaveConfig)
	HandleFunc("api/getConfig", handleGetConfig)

	// ensure we can listen without errors
	listener, err := net.Listen("tcp", cfg.Mod.Listen)
	if err != nil {
		log.Fatal().Err(err).Msg("[api] listen")
		return
	}

	log.Info().Str("addr", cfg.Mod.Listen).Msg("[api] listen")

	s := http.Server{}
	s.Handler = http.DefaultServeMux

	if log.Trace().Enabled() {
		s.Handler = middlewareLog(s.Handler)
	}

	if cfg.Mod.Origin == "*" {
		s.Handler = middlewareCORS(s.Handler)
	}

	go func() {
		if err = s.Serve(listener); err != nil {
			log.Fatal().Err(err).Msg("[api] serve")
		}
	}()
}

// HandleFunc handle pattern with relative path:
// - "api/streams" => "{basepath}/api/streams"
// - "/streams"    => "/streams"
func HandleFunc(pattern string, handler http.HandlerFunc) {
	if len(pattern) == 0 || pattern[0] != '/' {
		pattern = basePath + "/" + pattern
	}
	log.Trace().Str("path", pattern).Msg("[api] register path")
	http.HandleFunc(pattern, handler)
}

const StreamNotFound = "stream not found"

var basePath string
var log zerolog.Logger

func middlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Trace().Msgf("[api] %s %s", r.Method, r.URL)
		next.ServeHTTP(w, r)
	})
}

func middlewareCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		next.ServeHTTP(w, r)
	})
}
func handleGetConfig(w http.ResponseWriter, r *http.Request) {

	absPath := app.GetConfigPath()

	// Read the file contents.
	body, err := ioutil.ReadFile(absPath)

	if err != nil {
		w.Write([]byte(""))
		return
	}
	w.Write(body)
}

func handleSaveConfig(w http.ResponseWriter, r *http.Request) {
	absPath := app.GetConfigPath()

	// Save the file contents.
	body, _ := io.ReadAll(r.Body)

	var cfg struct {
		Mod map[string]string `yaml:"log"`
	}

	if body != nil {
		if err := yaml.Unmarshal(body, &cfg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	err := ioutil.WriteFile(absPath, []byte(body), 0644)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

}

func streamsHandler(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	name := r.URL.Query().Get("name")

	if name == "" {
		name = src
	}

	switch r.Method {
	case "PUT":
		streams.New(name, src)
		return
	case "DELETE":
		streams.Delete(src)
		return
	}

	var v interface{}
	if src != "" {
		v = streams.Get(src)
	} else {
		v = streams.All()
	}

	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	_ = e.Encode(v)
}
