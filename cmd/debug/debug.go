package debug

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"net/http"
	"os"
	"strconv"
)

func Init() {
	api.HandleFunc("api/stack", stackHandler)
	api.HandleFunc("api/exit", exitHandler)

	streams.HandleFunc("null", nullHandler)
}

func exitHandler(_ http.ResponseWriter, r *http.Request) {
	s := r.URL.Query().Get("code")
	code, _ := strconv.Atoi(s)
	os.Exit(code)
}

func nullHandler(string) (streamer.Producer, error) {
	return nil, nil
}
