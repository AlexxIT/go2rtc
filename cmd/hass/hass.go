package hass

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/app"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
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
