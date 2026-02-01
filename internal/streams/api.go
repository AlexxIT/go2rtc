package streams

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/creds"
	"github.com/AlexxIT/go2rtc/pkg/probe"
)

func apiStreams(w http.ResponseWriter, r *http.Request) {
	w = creds.SecretResponse(w)
	if api.IsReadOnly() {
		switch r.Method {
		case "PUT", "PATCH", "POST", "DELETE":
			api.ReadOnlyError(w)
			return
		}
	}

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

		cons := probe.Create("probe", query)
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

		if _, err := New(name, query["src"]...); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		if _, err := Patch(name, src); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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

	dot = []byte(creds.SecretString(string(dot)))

	api.Response(w, dot, "text/vnd.graphviz")
}

func apiPreload(w http.ResponseWriter, r *http.Request) {
	if api.IsReadOnly() {
		switch r.Method {
		case "PUT", "DELETE":
			api.ReadOnlyError(w)
			return
		}
	}
	// GET - return all preloads
	if r.Method == "GET" {
		api.ResponseJSON(w, GetPreloads())
		return
	}

	query := r.URL.Query()
	src := query.Get("src")

	switch r.Method {
	case "PUT":
		// it's safe to delete from map while iterating
		for k := range query {
			switch k {
			case core.KindVideo, core.KindAudio, "microphone":
			default:
				delete(query, k)
			}
		}

		rawQuery := query.Encode()

		if err := AddPreload(src, rawQuery); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := app.PatchConfig([]string{"preload", src}, rawQuery); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "DELETE":
		if err := DelPreload(src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := app.PatchConfig([]string{"preload", src}, nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
	}
}

func apiSchemes(w http.ResponseWriter, r *http.Request) {
	api.ResponseJSON(w, SupportedSchemes())
}
