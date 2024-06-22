package nest

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/nest"
)

func Init() {
	streams.HandleFunc("nest", func(source string) (core.Producer, error) {
		return nest.Dial(source)
	})

	api.HandleFunc("api/nest", apiNest)
}

func apiNest(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	cliendID := query.Get("client_id")
	cliendSecret := query.Get("client_secret")
	refreshToken := query.Get("refresh_token")
	projectID := query.Get("project_id")

	nestAPI, err := nest.NewAPI(cliendID, cliendSecret, refreshToken)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	devices, err := nestAPI.GetDevices(projectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []*api.Source

	for name, deviceID := range devices {
		query.Set("device_id", deviceID)

		items = append(items, &api.Source{
			Name: name, URL: "nest:?" + query.Encode(),
		})
	}

	api.ResponseSources(w, items)
}
