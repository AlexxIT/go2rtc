package hass

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/roborock"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/rs/zerolog"
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

	log = app.GetLogger("hass")

	initAPI()

	// support load cameras from Hass config file
	filename := path.Join(conf.Mod.Config, ".storage/core.config_entries")
	b, err := os.ReadFile(filename)
	if err != nil {
		return
	}

	storage := new(entries)
	if err = json.Unmarshal(b, storage); err != nil {
		return
	}

	urls := map[string]string{}

	streams.HandleFunc("hass", func(url string) (core.Producer, error) {
		if hurl := urls[url[5:]]; hurl != "" {
			return streams.GetProducer(hurl)
		}
		return nil, fmt.Errorf("can't get url: %s", url)
	})

	for _, entrie := range storage.Data.Entries {
		switch entrie.Domain {
		case "generic":
			var options struct {
				StreamSource string `json:"stream_source"`
			}
			if err = json.Unmarshal(entrie.Data, &options); err != nil {
				continue
			}
			urls[entrie.Title] = options.StreamSource

		case "homekit_controller":
			var data struct {
				ClientID      string `json:"iOSPairingId"`
				ClientPrivate string `json:"iOSDeviceLTSK"`
				ClientPublic  string `json:"iOSDeviceLTPK"`
				DeviceID      string `json:"AccessoryPairingID"`
				DevicePublic  string `json:"AccessoryLTPK"`
				DeviceHost    string `json:"AccessoryIP"`
				DevicePort    uint16 `json:"AccessoryPort"`
			}
			if err = json.Unmarshal(entrie.Data, &data); err != nil {
				continue
			}
			urls[entrie.Title] = fmt.Sprintf(
				"homekit://%s:%d?client_id=%s&client_private=%s%s&device_id=%s&device_public=%s",
				data.DeviceHost, data.DevicePort,
				data.ClientID, data.ClientPrivate, data.ClientPublic,
				data.DeviceID, data.DevicePublic,
			)

		case "roborock":
			_ = json.Unmarshal(entrie.Data, &roborock.Auth)

		default:
			continue
		}

		log.Info().Str("url", "hass:"+entrie.Title).Msg("[hass] load stream")
		//streams.Get("hass:" + entrie.Title)
	}
}

var log zerolog.Logger

type entries struct {
	Data struct {
		Entries []struct {
			Title   string          `json:"title"`
			Domain  string          `json:"domain"`
			Data    json.RawMessage `json:"data"`
			Options json.RawMessage `json:"options"`
		} `json:"entries"`
	} `json:"data"`
}
