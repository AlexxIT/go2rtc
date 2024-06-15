package hass

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
)

func apiOK(w http.ResponseWriter, r *http.Request) {
	api.Response(w, `{"status":1,"payload":{}}`, api.MimeJSON)
}

func apiStream(w http.ResponseWriter, r *http.Request) {
	switch {
	// /stream/{id}/add
	case strings.HasSuffix(r.RequestURI, "/add"):
		var v addJSON
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// we can get three types of links:
		// 1. link to go2rtc stream: rtsp://...:8554/{stream_name}
		// 2. static link to Hass camera
		// 3. dynamic link to Hass camera
		if streams.Patch(v.Name, v.Channels.First.Url) != nil {
			apiOK(w, r)
		} else {
			http.Error(w, "", http.StatusBadRequest)
		}

	// /stream/{id}/channel/0/webrtc
	default:
		i := strings.IndexByte(r.RequestURI[8:], '/')
		if i <= 0 {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		name := r.RequestURI[8 : 8+i]
		stream := streams.Get(name)
		if stream == nil {
			http.Error(w, api.StreamNotFound, http.StatusNotFound)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s := r.FormValue("data")
		offer, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s, err = webrtc.ExchangeSDP(stream, string(offer), "hass/webrtc", r.UserAgent())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		s = base64.StdEncoding.EncodeToString([]byte(s))
		_, _ = w.Write([]byte(s))
	}
}

func HassioAddr() string {
	ints, _ := net.Interfaces()

	for _, i := range ints {
		if i.Name != "hassio" {
			continue
		}

		addrs, _ := i.Addrs()
		for _, addr := range addrs {
			if addr, ok := addr.(*net.IPNet); ok {
				return addr.IP.String()
			}
		}
	}

	return ""
}

type addJSON struct {
	Name     string `json:"name"`
	Channels struct {
		First struct {
			//Name string `json:"name"`
			Url string `json:"url"`
		} `json:"0"`
	} `json:"channels"`
}
