package ring

import (
	"net/http"
	"net/url"

	"fmt"

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
	var ringAPI *ring.RingApi

	// Check auth method
	if email := query.Get("email"); email != "" {
		// Email/Password Flow
		password := query.Get("password")
		code := query.Get("code")

		var err error
		ringAPI, err = ring.NewRestClient(ring.EmailAuth{
			Email:    email,
			Password: password,
		}, nil)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Try authentication (this will trigger 2FA if needed)
		if _, err = ringAPI.GetAuth(code); err != nil {
			if ringAPI.Using2FA {
				// Return 2FA prompt
				api.ResponseJSON(w, map[string]interface{}{
					"needs_2fa": true,
					"prompt":    ringAPI.PromptFor2FA,
				})
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if refreshToken := query.Get("refresh_token"); refreshToken != "" {
		// Refresh Token Flow
		if refreshToken == "" {
			http.Error(w, "either email/password or refresh_token is required", http.StatusBadRequest)
			return
		}

		var err error
		ringAPI, err = ring.NewRestClient(ring.RefreshTokenAuth{
			RefreshToken: refreshToken,
		}, nil)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "either email/password or refresh token is required", http.StatusBadRequest)
		return
	}

	devices, err := ringAPI.FetchRingDevices()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cleanQuery := url.Values{}
	cleanQuery.Set("refresh_token", ringAPI.RefreshToken)

	var items []*api.Source
	for _, camera := range devices.AllCameras {
		cleanQuery.Set("camera_id", fmt.Sprint(camera.ID))
		cleanQuery.Set("device_id", camera.DeviceID)

		// Stream source
		items = append(items, &api.Source{
			Name: camera.Description,
			URL:  "ring:?" + cleanQuery.Encode(),
		})

		// Snapshot source
		items = append(items, &api.Source{
			Name: camera.Description + " Snapshot",
			URL:  "ring:?" + cleanQuery.Encode() + "&snapshot",
		})
	}

	api.ResponseSources(w, items)
}
