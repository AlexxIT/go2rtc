package ring

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/ring"
)

func Init() {
	streams.HandleFunc("ring", func(source string) (core.Producer, error) {
		return ring.Dial(source)
	})

	api.HandleFunc("api/ring", apiRing)
}

func apiRing(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	refreshToken := query.Get("refresh_token")

	ringAPI, err := ring.NewRingRestClient(ring.RefreshTokenAuth{RefreshToken: refreshToken}, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	devices, err := ringAPI.FetchRingDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []*api.Source

	for _, camera := range devices.AllCameras {
		query.Set("device_id", camera.DeviceID)

		items = append(items, &api.Source{
			Name: camera.Description, URL: "ring:?" + query.Encode(),
		})
	}

	api.ResponseSources(w, items)
}
