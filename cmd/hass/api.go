package hass

import (
	"encoding/base64"
	"encoding/json"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/cmd/webrtc"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func initAPI() {
	ok := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"payload":{}}`))
	}

	// support https://www.home-assistant.io/integrations/rtsp_to_webrtc/
	api.HandleFunc("/static", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	api.HandleFunc("/streams", ok)

	api.HandleFunc("/stream/", func(w http.ResponseWriter, r *http.Request) {
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
				// check if it is rtsp link to go2rtc
				stream = rtspStream(v.Channels.First.Url)
				if stream != nil {
					streams.New(v.Name, stream)
				} else {
					stream = streams.New(v.Name, "{input}")
				}
			}

			stream.SetSource(v.Channels.First.Url)

			ok(w, r)

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
	})

	// api from RTSPtoWebRTC
	api.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			return
		}

		str := r.FormValue("sdp64")
		offer, err := base64.StdEncoding.DecodeString(str)
		if err != nil {
			return
		}

		src := r.FormValue("url")
		src, err = url.QueryUnescape(src)
		if err != nil {
			return
		}

		stream := streams.Get(src)
		if stream == nil {
			if stream = rtspStream(src); stream != nil {
				streams.New(src, stream)
			} else {
				stream = streams.New(src, src)
			}
		}

		str, err = webrtc.ExchangeSDP(stream, string(offer), "WebRTC/Hass sync", r.UserAgent())
		if err != nil {
			return
		}

		v := struct {
			Answer string `json:"sdp64"`
		}{
			Answer: base64.StdEncoding.EncodeToString([]byte(str)),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	})
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

func rtspStream(url string) *streams.Stream {
	if strings.HasPrefix(url, "rtsp://") {
		if i := strings.IndexByte(url[7:], '/'); i > 0 {
			return streams.Get(url[8+i:])
		}
	}
	return nil
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
