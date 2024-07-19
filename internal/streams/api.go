package streams

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/probe"
)

func apiStreams(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	srcs := query["src"]
	name := query.Get("name")

	if name == "" && len(srcs) == 0 {
		api.ResponseJSON(w, streams)
		return
	}

	switch r.Method {
	case "GET":
		if len(srcs) == 0 {
			http.Error(w, "Query 'src' is required", http.StatusBadRequest)
			return
		}

		stream := Get(srcs[0])
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
			api.ResponsePrettyJSON(w, streams[srcs[0]])
		}

	case "PUT":
		if name == "" {
			http.Error(w, "Query 'name' is required", http.StatusBadRequest)
			return
		}

		if streams[name] != nil {
			http.Error(w, "Stream already exists", http.StatusConflict)
			return
		}

		if New(name, srcs) == nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		if err := app.PatchConfig(name, srcs, "streams"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case "PATCH":
		if name == "" {
			http.Error(w, "Query 'name' is required", http.StatusBadRequest)
			return
		}

		if streams[name] == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		// support {input} templates: https://github.com/AlexxIT/go2rtc#module-hass
		if Patch(name, srcs...) == nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		if err := app.PatchConfig(name, srcs, "streams"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}

	case "POST":
		if len(srcs) == 0 {
			http.Error(w, "Query 'src' is required", http.StatusBadRequest)
			return
		}

		// with dst - redirect source to dst
		if dst := query.Get("dst"); dst != "" {
			if stream := Get(dst); stream != nil {
				if err := Validate(srcs[0]); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
				} else if err = stream.Play(srcs[0]); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				} else {
					api.ResponseJSON(w, stream)
				}
			} else if stream = Get(srcs[0]); stream != nil {
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
		if name == "" {
			http.Error(w, "Query 'name' is required", http.StatusBadRequest)
			return
		}

		if streams[name] == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		delete(streams, name)

		if err := app.PatchConfig(name, nil, "streams"); err != nil {
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
