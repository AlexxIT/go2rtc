package api

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/shell"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Listen    string `yaml:"listen"`
			Username  string `yaml:"username"`
			Password  string `yaml:"password"`
			BasePath  string `yaml:"base_path"`
			StaticDir string `yaml:"static_dir"`
			Origin    string `yaml:"origin"`
			TLSListen string `yaml:"tls_listen"`
			TLSCert   string `yaml:"tls_cert"`
			TLSKey    string `yaml:"tls_key"`
		} `yaml:"api"`
	}

	// default config
	cfg.Mod.Listen = "0.0.0.0:1984"

	// load config from YAML
	app.LoadConfig(&cfg)

	if cfg.Mod.Listen == "" {
		return
	}

	basePath = cfg.Mod.BasePath
	log = app.GetLogger("api")

	initStatic(cfg.Mod.StaticDir)

	HandleFunc("api", apiHandler)
	HandleFunc("api/config", configHandler)
	HandleFunc("api/exit", exitHandler)
	HandleFunc("api/restart", restartHandler)

	// ensure we can listen without errors
	var err error
	ln, err = net.Listen("tcp", cfg.Mod.Listen)
	if err != nil {
		log.Fatal().Err(err).Msg("[api] listen")
		return
	}

	log.Info().Str("addr", cfg.Mod.Listen).Msg("[api] listen")

	Handler = http.DefaultServeMux // 4th

	if cfg.Mod.Origin == "*" {
		Handler = middlewareCORS(Handler) // 3rd
	}

	if cfg.Mod.Username != "" {
		Handler = middlewareAuth(cfg.Mod.Username, cfg.Mod.Password, Handler) // 2nd
	}

	if log.Trace().Enabled() {
		Handler = middlewareLog(Handler) // 1st
	}

	go func() {
		s := http.Server{}
		s.Handler = Handler
		if err = s.Serve(ln); err != nil {
			log.Fatal().Err(err).Msg("[api] serve")
		}
	}()

	// Initialize the HTTPS server
	if cfg.Mod.TLSListen != "" && cfg.Mod.TLSCert != "" && cfg.Mod.TLSKey != "" {
		cert, err := tls.X509KeyPair([]byte(cfg.Mod.TLSCert), []byte(cfg.Mod.TLSKey))
		if err != nil {
			log.Error().Err(err).Caller().Send()
			return
		}

		tlsListener, err := net.Listen("tcp", cfg.Mod.TLSListen)
		if err != nil {
			log.Fatal().Err(err).Caller().Send()
			return
		}

		log.Info().Str("addr", cfg.Mod.TLSListen).Msg("[api] tls listen")

		tlsServer := &http.Server{
			Handler: Handler,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}

		go func() {
			if err := tlsServer.ServeTLS(tlsListener, "", ""); err != nil {
				log.Fatal().Err(err).Msg("[api] tls serve")
			}
		}()
	}
}

func Port() int {
	if ln == nil {
		return 0
	}
	return ln.Addr().(*net.TCPAddr).Port
}

const (
	MimeJSON = "application/json"
	MimeText = "text/plain"
)

var Handler http.Handler

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

// ResponseJSON important always add Content-Type
// so go won't need to call http.DetectContentType
func ResponseJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", MimeJSON)
	_ = json.NewEncoder(w).Encode(v)
}

func ResponsePrettyJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", MimeJSON)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func Response(w http.ResponseWriter, body any, contentType string) {
	w.Header().Set("Content-Type", contentType)

	switch v := body.(type) {
	case []byte:
		_, _ = w.Write(v)
	case string:
		_, _ = w.Write([]byte(v))
	default:
		_, _ = fmt.Fprint(w, body)
	}
}

const StreamNotFound = "stream not found"

var basePath string
var log zerolog.Logger

func middlewareLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Trace().Msgf("[api] %s %s %s", r.Method, r.URL, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func middlewareAuth(username, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.RemoteAddr, "127.") && !strings.HasPrefix(r.RemoteAddr, "[::1]") {
			user, pass, ok := r.BasicAuth()
			if !ok || user != username || pass != password {
				w.Header().Set("Www-Authenticate", `Basic realm="go2rtc"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func middlewareCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization")
		next.ServeHTTP(w, r)
	})
}

var ln net.Listener
var mu sync.Mutex

func apiHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	app.Info["host"] = r.Host
	mu.Unlock()

	ResponseJSON(w, app.Info)
}

func exitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	s := r.URL.Query().Get("code")
	code, _ := strconv.Atoi(s)
	os.Exit(code)
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	go shell.Restart()
}

type Source struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Info     string `json:"info,omitempty"`
	URL      string `json:"url,omitempty"`
	Location string `json:"location,omitempty"`
}

func ResponseSources(w http.ResponseWriter, sources []*Source) {
	if len(sources) == 0 {
		http.Error(w, "no sources", http.StatusNotFound)
		return
	}

	var response = struct {
		Sources []*Source `json:"sources"`
	}{
		Sources: sources,
	}
	ResponseJSON(w, response)
}

func Error(w http.ResponseWriter, err error) {
	log.Error().Err(err).Caller(1).Send()

	http.Error(w, err.Error(), http.StatusInsufficientStorage)
}
