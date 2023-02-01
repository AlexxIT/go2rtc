package streams

import (
	"encoding/json"
	"net/http"

	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/app/store"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod map[string]interface{} `yaml:"streams"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("streams")

	for name, item := range cfg.Mod {
		streams[name] = NewStream(item)
	}

	for name, item := range store.GetDict("streams") {
		streams[name] = NewStream(item)
	}

	api.HandleFunc("api/streams", streamsHandler)
}

func Get(name string) *Stream {
	return streams[name]
}

func New(name string, source interface{}) *Stream {
	stream := NewStream(source)
	streams[name] = stream
	return stream
}

func GetOrNew(src string) *Stream {
	if stream, ok := streams[src]; ok {
		return stream
	}

	if !HasProducer(src) {
		return nil
	}

	log.Info().Str("url", src).Msg("[streams] create new stream")

	return New(src, src)
}

func streamsHandler(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")

	switch r.Method {
	case "PUT":
		name := r.URL.Query().Get("name")
		if name == "" {
			name = src
		}
		New(name, src)
		if err := storeStreams(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	case "DELETE":
		delete(streams, src)
		if err := storeStreams(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	if src != "" {
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		_ = e.Encode(streams[src])
	} else {
		_ = json.NewEncoder(w).Encode(streams)
	}
}

var log zerolog.Logger
var streams = map[string]*Stream{}
