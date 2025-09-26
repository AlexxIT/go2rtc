package streams

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/probe"
)

func apiStreams(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	src := query.Get("src")

	// without source - return all streams list
	if src == "" && r.Method != "POST" {
		api.ResponseJSON(w, streams)
		return
	}

	// Not sure about all this API. Should be rewrited...
	switch r.Method {
	case "GET":
		stream := Get(src)
		if stream == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		cons := probe.NewProbe(query)
		if len(cons.Medias) != 0 {
			cons.WithRequest(r)
			if err := stream.AddConsumer(cons); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			api.ResponsePrettyJSON(w, stream)

			stream.RemoveConsumer(cons)
		} else {
			api.ResponsePrettyJSON(w, streams[src])
		}

	case "PUT":
		name := query.Get("name")
		if name == "" {
			name = src
		}

		if New(name, query["src"]...) == nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		if err := app.PatchConfig([]string{"streams", name}, query["src"]); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case "PATCH":
		name := query.Get("name")
		if name == "" {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		// support {input} templates: https://github.com/AlexxIT/go2rtc#module-hass
		if Patch(name, src) == nil {
			http.Error(w, "", http.StatusBadRequest)
		}

	case "POST":
		// with dst - redirect source to dst
		if dst := query.Get("dst"); dst != "" {
			if stream := Get(dst); stream != nil {
				if err := Validate(src); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else if err = stream.Play(src); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else {
					api.ResponseJSON(w, stream)
				}
			} else if stream = Get(src); stream != nil {
				if err := Validate(dst); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else if err = stream.Publish(dst); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			} else {
				http.Error(w, "", http.StatusNotFound)
			}
		} else {
			http.Error(w, "", http.StatusBadRequest)
		}

	case "DELETE":
		delete(streams, src)

		if err := app.PatchConfig([]string{"streams", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func apiStreamsDOT(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	dot := make([]byte, 0, 1024)
	dot = append(dot, "digraph {\n"...)
	if query.Has("src") {
		for _, name := range query["src"] {
			if stream := streams[name]; stream != nil {
				dot = AppendDOT(dot, stream)
			}
		}
	} else {
		for _, stream := range streams {
			dot = AppendDOT(dot, stream)
		}
	}
	dot = append(dot, '}')

	api.Response(w, dot, "text/vnd.graphviz")
}

func apiPreload(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	src := query.Get("src")
	query.Del("src")

	if src == "" {
		http.Error(w, "no source", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "PUT":
		// check if stream exists
		stream := Get(src)
		if stream == nil {
			http.Error(w, "stream not found", http.StatusNotFound)
			return
		}

		// check if consumer exists
		if cons, ok := preloads[src]; ok {
			stream.RemoveConsumer(cons)
			delete(preloads, src)
		}

		// parse query parameters
		var rawQuery string
		if query.Has("video") {
			if videoQuery := query.Get("video"); videoQuery != "" {
				rawQuery += "video=" + videoQuery + "#"
			} else {
				rawQuery += "video#"
			}
		}
		if query.Has("audio") {
			if audioQuery := query.Get("audio"); audioQuery != "" {
				rawQuery += "audio=" + audioQuery + "#"
			} else {
				rawQuery += "audio#"
			}
		}
		if query.Has("microphone") {
			if micQuery := query.Get("microphone"); micQuery != "" {
				rawQuery += "microphone=" + micQuery + "#"
			} else {
				rawQuery += "microphone#"
			}
		}

		if err := app.PatchConfig([]string{"preload", src}, rawQuery); err != nil {
			log.Error().Err(err).Str("src", src).Msg("Failed to patch config for PUT")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		Preload(src, rawQuery)

	case "DELETE":
		if cons, ok := preloads[src]; ok {
			if stream := Get(src); stream != nil {
				stream.RemoveConsumer(cons)
			} else {
				cons.Stop()
			}

			delete(preloads, src)
		}

		if err := app.PatchConfig([]string{"preload", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
	}
}
