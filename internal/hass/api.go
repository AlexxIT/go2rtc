package hass

import (
	"encoding/base64"
	"encoding/json"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/webrtc"
	"net"
	"net/http"
	"strings"
)

func apiOK(w http.ResponseWriter, r *http.Request) {
	api.ResponseRawJSON(w, `{"status":1,"payload":{}}`)
}

func apiStream(w http.ResponseWriter, r *http.Request) {
	switch {
	// /stream/{id}/add
	case strings.HasSuffix(r.RequestURI, "/add"):
		var v addJSON
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			return
		}

		// we can get three types of links:
		// 1. link to go2rtc stream: rtsp://...:8554/{stream_name}
		// 2. static link to Hass camera
		// 3. dynamic link to Hass camera
		stream := streams.Get(v.Name)
		if stream == nil {
			stream = streams.NewTemplate(v.Name, v.Channels.First.Url)
		}

		stream.SetSource(v.Channels.First.Url)

		apiOK(w, r)

	// /stream/{id}/channel/0/webrtc
	default:
		i := strings.IndexByte(r.RequestURI[8:], '/')
		if i <= 0 {
			log.Warn().Msgf("wrong request: %s", r.RequestURI)
			return
		}
		name := r.RequestURI[8 : 8+i]

		stream := streams.Get(name)
		if stream == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if err := r.ParseForm(); err != nil {
			log.Error().Err(err).Msg("[api.hass] parse form")
			return
		}

		s := r.FormValue("data")
		offer, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			log.Error().Err(err).Msg("[api.hass] sdp64 decode")
			return
		}

		s, err = webrtc.ExchangeSDP(stream, string(offer), "WebRTC/Hass sync", r.UserAgent())
		if err != nil {
			log.Error().Err(err).Msg("[api.hass] exchange SDP")
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
