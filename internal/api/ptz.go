package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/onvif"
)

type PTZCommand struct {
	Source string  `json:"src"`
	Action string  `json:"action"`
	Pan    float64 `json:"pan,omitempty"`
	Tilt   float64 `json:"tilt,omitempty"`
	Zoom   float64 `json:"zoom,omitempty"`
}

// These variables will be set by the streams package to avoid import cycles
var (
	// GetStreamInfo returns stream source URLs for a given stream name
	GetStreamInfo func(name string) []string
)

func ptzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cmd PTZCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get actual camera URL from stream name if it's not a direct URL
	src := cmd.Source

	var cfg struct {
		Streams map[string]any `yaml:"streams"`
	}

	app.LoadConfig(&cfg)

	for name, item := range cfg.Streams {
		if name == src {
			switch v := item.(type) {
			case string:
				src = v
			case []interface{}:
				// Handle array/slice type, get the first element
				if len(v) > 0 {
					src = fmt.Sprint(v[0])
				}
			case []string:
				// Handle string array/slice
				if len(v) > 0 {
					src = v[0]
				}
			default:
				// For any other type, convert to string
				src = fmt.Sprint(item)
			}
			break
		}
	}

	src = strings.TrimPrefix(src, "[")
	src = strings.TrimSuffix(src, "]")

	log.Debug().Str("url", src).Msg("[api] connecting to ONVIF device")
	client, err := onvif.NewClient(src)
	if err != nil {
		log.Error().Err(err).Str("url", src).Msg("[api] failed to create ONVIF client")
		http.Error(w, "Failed to create ONVIF client: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tokens, err := client.GetProfilesTokens()
	if err != nil || len(tokens) == 0 {
		log.Error().Err(err).Str("url", src).Msg("[api] failed to get profile tokens")
		http.Error(w, "Failed to get profile tokens", http.StatusInternalServerError)
		return
	}

	log.Debug().Strs("tokens", tokens).Msg("[api] retrieved profile tokens")

	var resp []byte
	switch cmd.Action {
	case "move":
		resp, err = client.PTZRequest(onvif.PTZContinuousMove, tokens[0],
			formatSpeed(cmd.Pan), formatSpeed(cmd.Tilt), formatSpeed(cmd.Zoom))
	case "stop":
		resp, err = client.PTZRequest(onvif.PTZStop, tokens[0])
	default:
		log.Error().Str("action", cmd.Action).Msg("[api] unsupported PTZ action")
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	log.Debug().Str("resp", string(resp)).Msg("[api] Raw response")

	if err != nil {
		log.Error().Err(err).Str("action", cmd.Action).Msg("[api] PTZ command failed")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func formatSpeed(speed float64) string {
	if speed < -1 {
		speed = -1
	} else if speed > 1 {
		speed = 1
	}
	return fmt.Sprintf("%.2f", speed)
}

func init() {
	HandleFunc("api/ptz", ptzHandler)
}
