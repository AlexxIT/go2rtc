package api

import (
	"github.com/AlexxIT/go2rtc/cmd/app"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
)

func configHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		data, err := os.ReadFile(app.ConfigPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if _, err = w.Write(data); err != nil {
			log.Warn().Err(err).Caller().Send()
		}

	case "POST":
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// validate config
		var tmp struct{}
		if err = yaml.Unmarshal(data, &tmp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err = os.WriteFile(app.ConfigPath, data, 0644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
