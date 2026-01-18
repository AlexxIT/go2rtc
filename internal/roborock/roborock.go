package roborock

import (
	"fmt"
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/roborock"
)

func Init() {
	streams.HandleFunc("roborock", func(source string) (core.Producer, error) {
		return roborock.Dial(source)
	})

	api.HandleFunc("api/roborock", apiHandle)
}

var Auth struct {
	UserData *roborock.UserInfo `json:"user_data"`
	BaseURL  string             `json:"base_url"`
}

func apiHandle(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if Auth.UserData == nil {
			http.Error(w, "no auth", http.StatusNotFound)
			return
		}

	case "POST":
		if err := r.ParseMultipartForm(1024); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		username := r.Form.Get("username")
		password := r.Form.Get("password")
		if username == "" || password == "" {
			http.Error(w, "empty username or password", http.StatusBadRequest)
			return
		}

		base, err := roborock.GetBaseURL(username)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		ui, err := roborock.Login(base, username, password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		Auth.BaseURL = base
		Auth.UserData = ui

	default:
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	homeID, err := roborock.GetHomeID(Auth.BaseURL, Auth.UserData.Token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	devices, err := roborock.GetDevices(Auth.UserData, homeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []*api.Source

	for _, device := range devices {
		source := fmt.Sprintf(
			"roborock://%s?u=%s&s=%s&k=%s&did=%s&key=%s&pin=",
			Auth.UserData.IoT.URL.MQTT[6:],
			Auth.UserData.IoT.User, Auth.UserData.IoT.Pass, Auth.UserData.IoT.Domain,
			device.DID, device.Key,
		)
		items = append(items, &api.Source{Name: device.Name, URL: source})
	}

	api.ResponseSources(w, items)
}
