package api

import (
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"

	"github.com/AlexxIT/go2rtc/internal/api/server"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/rs/zerolog"
)

func Init() {
	server.Init()
	log = app.GetLogger("api")

	server.HandleFunc("api", apiHandler)
	server.HandleFunc("api/config", configHandler)
	server.HandleFunc("api/exit", exitHandler)
	server.HandleFunc("api/restart", restartHandler)
	server.HandleFunc("api/log", logHandler)
}

var log zerolog.Logger

var mu sync.Mutex

func apiHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	app.Info["host"] = r.Host
	mu.Unlock()

	server.ResponseJSON(w, app.Info)
}

func exitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	s := r.URL.Query().Get("code")
	code, err := strconv.Atoi(s)

	// https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#tag_18_08_02
	if err != nil || code < 0 || code > 125 {
		http.Error(w, "Code must be in the range [0, 125]", http.StatusBadRequest)
		return
	}

	os.Exit(code)
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	path, err := os.Executable()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msgf("[api] restart %s", path)

	go syscall.Exec(path, os.Args, os.Environ())
}

func logHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		// Send current state of the log file immediately
		w.Header().Set("Content-Type", "application/jsonlines")
		_, _ = app.MemoryLog.WriteTo(w)
	case "DELETE":
		app.MemoryLog.Reset()
		server.Response(w, "OK", "text/plain")
	default:
		http.Error(w, "Method not allowed", http.StatusBadRequest)
	}
}
