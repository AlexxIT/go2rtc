package hass

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/roborock"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"path"
)

func Init() {
	var conf struct {
		API struct {
			Listen string `json:"listen"`
		} `yaml:"api"`
		Mod struct {
			Config string `yaml:"config"`
		} `yaml:"hass"`
	}

	app.LoadConfig(&conf)

	log = app.GetLogger("hass")

	initAPI()

	entries := importEntries(conf.Mod.Config)
	if entries == nil {
		api.HandleFunc("api/hass", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no hass config", http.StatusNotFound)
		})
		return
	}

	api.HandleFunc("api/hass", func(w http.ResponseWriter, _ *http.Request) {
		var items []api.Stream
		for name, url := range entries {
			items = append(items, api.Stream{Name: name, URL: url})
		}
		api.ResponseStreams(w, items)
	})

	streams.HandleFunc("hass", func(url string) (core.Producer, error) {
		if hurl := entries[url[5:]]; hurl != "" {
			return streams.GetProducer(hurl)
		}
		return nil, fmt.Errorf("can't get url: %s", url)
	})

	// for Addon listen on hassio interface, so WebUI feature will work
	if conf.API.Listen == "127.0.0.1:1984" {
		if addr := HassioAddr(); addr != "" {
			addr += ":1984"
			go func() {
				log.Info().Str("addr", addr).Msg("[hass] listen")
				if err := http.ListenAndServe(addr, api.Handler); err != nil {
					log.Error().Err(err).Caller().Send()
				}
			}()
		}
	}
}

func importEntries(config string) map[string]string {
	// support load cameras from Hass config file
	filename := path.Join(config, ".storage/core.config_entries")
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}

	var storage struct {
		Data struct {
			Entries []struct {
				Title   string          `json:"title"`
				Domain  string          `json:"domain"`
				Data    json.RawMessage `json:"data"`
				Options json.RawMessage `json:"options"`
			} `json:"entries"`
		} `json:"data"`
	}

	if err = json.Unmarshal(b, &storage); err != nil {
		return nil
	}

	urls := map[string]string{}

	for _, entrie := range storage.Data.Entries {
		switch entrie.Domain {
		case "generic":
			var options struct {
				StreamSource string `json:"stream_source"`
			}
			if err = json.Unmarshal(entrie.Options, &options); err != nil {
				continue
			}
			urls[entrie.Title] = options.StreamSource

		case "homekit_controller":
			if !bytes.Contains(entrie.Data, []byte("iOSPairingId")) {
				continue
			}

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

		case "onvif":
			var data struct {
				Host     string `json:"host" json:"host"`
				Port     uint16 `json:"port" json:"port"`
				Username string `json:"username" json:"username"`
				Password string `json:"password" json:"password"`
			}
			if err = json.Unmarshal(entrie.Data, &data); err != nil {
				continue
			}

			if data.Username != "" && data.Password != "" {
				urls[entrie.Title] = fmt.Sprintf(
					"onvif://%s:%s@%s:%d", data.Username, data.Password, data.Host, data.Port,
				)
			} else {
				urls[entrie.Title] = fmt.Sprintf("onvif://%s:%d", data.Host, data.Port)
			}

		default:
			continue
		}

		log.Info().Str("url", "hass:"+entrie.Title).Msg("[hass] load stream")
		//streams.Get("hass:" + entrie.Title)
	}

	return urls
}

var log zerolog.Logger
