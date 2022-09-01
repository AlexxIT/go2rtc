package hass

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/rtsp"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/cmd/webrtc"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"path"
	"strings"
)

func Init() {
	var conf struct {
		Mod struct {
			Config string `yaml:"config"`
		} `yaml:"hass"`
	}

	app.LoadConfig(&conf)

	log = app.GetLogger("api")

	// support https://www.home-assistant.io/integrations/rtsp_to_webrtc/
	api.HandleFunc("/static", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	api.HandleFunc("/stream", handler)

	// support load cameras from Hass config file
	filename := path.Join(conf.Mod.Config, ".storage/core.config_entries")
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	storage := new(entries)
	if err = json.Unmarshal(data, storage); err != nil {
		return
	}

	urls := map[string]string{}

	streams.HandleFunc("hass", func(url string) (streamer.Producer, error) {
		if hurl := urls[url[5:]]; hurl != "" {
			return streams.GetProducer(hurl)
		}
		return nil, fmt.Errorf("can't get url: %s", url)
	})

	for _, entrie := range storage.Data.Entries {
		switch entrie.Domain {
		case "generic":
			if entrie.Options.StreamSource == "" {
				continue
			}
			urls[entrie.Title] = entrie.Options.StreamSource

		case "homekit_controller":
			if entrie.Data.ClientID == "" {
				continue
			}
			urls[entrie.Title] = fmt.Sprintf(
				"homekit://%s:%d?client_id=%s&client_private=%s%s&device_id=%s&device_public=%s",
				entrie.Data.DeviceHost, entrie.Data.DevicePort,
				entrie.Data.ClientID, entrie.Data.ClientPrivate, entrie.Data.ClientPublic,
				entrie.Data.DeviceID, entrie.Data.DevicePublic,
			)

		default:
			continue
		}

		streams.Get("hass:" + entrie.Title)
	}
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

	// TODO: fixme
	if strings.HasPrefix(url, "rtsp://") {
		port := ":" + rtsp.Port + "/"
		i := strings.Index(url, port)
		if i > 0 {
			url = url[i+len(port):]
		}
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
