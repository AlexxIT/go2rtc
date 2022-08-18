package hass

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/cmd/webrtc"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"path"
)

func Init() {
	var conf struct {
		Mod struct {
			Config string `yaml:"config"`
		} `yaml:"hass"`
	}

	app.LoadConfig(&conf)

	filename := path.Join(conf.Mod.Config, ".storage/core.config_entries")
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	ent := new(entries)
	if err = json.Unmarshal(data, ent); err != nil {
		return
	}

	urls := map[string]string{}

	for _, entrie := range ent.Data.Entries {
		switch entrie.Domain {
		case "generic":
			if entrie.Options.StreamSource != "" {
				urls[entrie.Title] = entrie.Options.StreamSource
			}
		}
	}

	streams.HandleFunc("hass", func(url string) (streamer.Producer, error) {
		if hurl := urls[url[5:]]; hurl != "" {
			return streams.GetProducer(hurl)
		}
		return nil, fmt.Errorf("can't get url: %s", url)
	})

	// support https://www.home-assistant.io/integrations/rtsp_to_webrtc/
	api.HandleFunc("/static", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	api.HandleFunc("/stream", handler)

	log = app.GetLogger("api")
}

var log zerolog.Logger

func handler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		log.Error().Err(err).Msg("[api.hass] parse form")
		return
	}

	url := r.FormValue("url")
	str := r.FormValue("sdp64")

	offer, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		log.Error().Err(err).Msg("[api.hass] sdp64 decode")
		return
	}

	stream := streams.Get(url)
	str, err = webrtc.ExchangeSDP(stream, string(offer), r.UserAgent())
	if err != nil {
		log.Error().Err(err).Msg("[api.hass] exchange SDP")
		return
	}

	resp := struct {
		Answer string `json:"sdp64"`
	}{
		Answer: base64.StdEncoding.EncodeToString([]byte(str)),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Error().Err(err).Msg("[api.hass] marshal JSON")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err = w.Write(data); err != nil {
		log.Error().Err(err).Msg("[api.hass] write")
		return
	}
}

type entries struct {
	Data struct {
		Entries []struct {
			Title  string `json:"title"`
			Domain string `json:"domain"`
			Data   struct {
				ClientID      string `json:"iOSPairingId"`
				ClientPrivate string `json:"iOSDeviceLTSK"`
				ClientPublic  string `json:"iOSDeviceLTPK"`
				DeviceID      string `json:"AccessoryPairingID"`
				DevicePublic  string `json:"AccessoryLTPK"`
				DeviceHost    string `json:"AccessoryIP"`
				DevicePort    uint16 `json:"AccessoryPort"`
			} `json:"data"`
			Options struct {
				StreamSource string `json:"stream_source"`
			}
		} `json:"entries"`
	} `json:"data"`
}
