package hass

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
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

		log.Info().Str("url", "hass:"+entrie.Title).Msg("[hass] load stream")
		//streams.Get("hass:" + entrie.Title)
	}
}

var log zerolog.Logger

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
