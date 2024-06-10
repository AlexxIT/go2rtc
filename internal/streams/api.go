package streams

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/probe"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
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
			cons.RemoteAddr = tcp.RemoteAddr(r)
			cons.UserAgent = r.UserAgent()
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

		if New(name, src) == nil {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		if err := app.PatchConfig(name, src, "streams"); err != nil {
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

		if err := app.PatchConfig(src, nil, "streams"); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}
