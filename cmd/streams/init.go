package streams

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/app/store"
	"github.com/rs/zerolog"
	"net/http"
	"net/url"
)

func Init() {
	var cfg struct {
		Mod map[string]any `yaml:"streams"`
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

func New(name string, source any) *Stream {
	stream := NewStream(source)
	streams[name] = stream
	return stream
}

func NewTemplate(name string, source any) *Stream {
	// check if source links to some stream name from go2rtc
	if rawURL, ok := source.(string); ok {
		if u, err := url.Parse(rawURL); err == nil && u.Scheme == "rtsp" {
			if stream, ok := streams[u.Path[1:]]; ok {
				streams[name] = stream
				return stream
			}
		}
	}

	return New(name, "{input}")
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

func GetAll() (names []string) {
	for name := range streams {
		names = append(names, name)
	}
	return
}

func streamsHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	src := query.Get("src")

	// without source - return all streams list
	if src == "" && r.Method != "POST" {
		_ = json.NewEncoder(w).Encode(streams)
		return
	}

	// Not sure about all this API. Should be rewrited...
	switch r.Method {
	case "GET":
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		_ = e.Encode(streams[src])

	case "PUT":
		name := query.Get("name")
		if name == "" {
			name = src
		}

		New(name, src)

	case "PATCH":
		name := query.Get("name")
		if name == "" {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		// support {input} templates: https://github.com/AlexxIT/go2rtc#module-hass
		stream := Get(name)
		if stream == nil {
			stream = NewTemplate(name, src)
		}
		stream.SetSource(src)

	case "POST":
		// with dst - redirect source to dst
		if dst := query.Get("dst"); dst != "" {
			if stream := Get(dst); stream != nil {
				if err := stream.Play(src); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else {
					_ = json.NewEncoder(w).Encode(stream)
				}
			} else {
				http.Error(w, "", http.StatusNotFound)
			}
		} else {
			http.Error(w, "", http.StatusBadRequest)
		}

	case "DELETE":
		delete(streams, src)
	}
}

var log zerolog.Logger
var streams = map[string]*Stream{}
