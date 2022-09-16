package hass

import (
	"encoding/base64"
	"encoding/json"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/cmd/webrtc"
	"net/http"
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

			if streams.Has(v.Name) {
				stream := streams.Get(v.Name)
				stream.SetSource(v.Channels.First.Url)
			} else {
				streams.New(v.Name, v.Channels.First.Url)
			}

			ok(w, r)

		// /stream/{id}/channel/0/webrtc
		default:
			i := strings.IndexByte(r.RequestURI[8:], '/')
			src := r.RequestURI[8 : 8+i]
			if !streams.Has(src) {
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

			// check if stream links to our rtsp server
			//if strings.HasPrefix(src, "rtsp://") {
			//	i := strings.IndexByte(src[7:], '/')
			//	if i > 0 && streams.Has(src[8+i:]) {
			//		src = src[8+i:]
			//	}
			//}

			stream := streams.Get(src)
			s, err = webrtc.ExchangeSDP(stream, string(offer), r.UserAgent())
			if err != nil {
				log.Error().Err(err).Msg("[api.hass] exchange SDP")
				return
			}

			s = base64.StdEncoding.EncodeToString([]byte(s))
			_, _ = w.Write([]byte(s))
		}
	})
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
