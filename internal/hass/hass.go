package hass

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/roborock"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hass"
	"github.com/rs/zerolog"
)

func Init() {
	var conf struct {
		API struct {
			Listen string `yaml:"listen"`
		} `yaml:"api"`
		Mod struct {
			Config string `yaml:"config"`
		} `yaml:"hass"`
	}

	app.LoadConfig(&conf)

	log = app.GetLogger("hass")

	// support API for https://www.home-assistant.io/integrations/rtsp_to_webrtc/
	api.HandleFunc("/static", apiOK)
	api.HandleFunc("/streams", apiOK)
	api.HandleFunc("/stream/", apiStream)

	streams.RedirectFunc("hass", func(url string) (string, error) {
		if location := entities[url[5:]]; location != "" {
			return location, nil
		}

		return "", nil
	})

	streams.HandleFunc("hass", func(source string) (core.Producer, error) {
		// support hass://supervisor?entity_id=camera.driveway_doorbell
		return hass.NewClient(source)
	})

	// load static entries from Hass config
	if err := importConfig(conf.Mod.Config); err != nil {
		log.Trace().Msgf("[hass] can't import config: %s", err)

		api.HandleFunc("api/hass", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no hass config", http.StatusNotFound)
		})
		return
	}

	api.HandleFunc("api/hass", func(w http.ResponseWriter, _ *http.Request) {
		once.Do(func() {
			// load WebRTC entities from Hass API, works only for add-on version
			if token := hass.SupervisorToken(); token != "" {
				if err := importWebRTC(token); err != nil {
					log.Warn().Err(err).Caller().Send()
				}
			}
		})

		var items []*api.Source
		for name, url := range entities {
			items = append(items, &api.Source{
				Name: name, URL: "hass:" + name, Location: url,
			})
		}
		api.ResponseSources(w, items)
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

func importConfig(config string) error {
	// support load cameras from Hass config file
	filename := path.Join(config, ".storage/core.config_entries")
	b, err := os.ReadFile(filename)
	if err != nil {
		return err
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
		return err
	}

	for _, entrie := range storage.Data.Entries {
		switch entrie.Domain {
		case "generic":
			var options struct {
				StreamSource string `json:"stream_source"`
			}
			if err = json.Unmarshal(entrie.Options, &options); err != nil {
				continue
			}
			entities[entrie.Title] = options.StreamSource

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
			entities[entrie.Title] = fmt.Sprintf(
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
				entities[entrie.Title] = fmt.Sprintf(
					"onvif://%s:%s@%s:%d", data.Username, data.Password, data.Host, data.Port,
				)
			} else {
				entities[entrie.Title] = fmt.Sprintf("onvif://%s:%d", data.Host, data.Port)
			}

		default:
			continue
		}

		log.Debug().Str("url", "hass:"+entrie.Title).Msg("[hass] load config")
		//streams.Get("hass:" + entrie.Title)
	}

	return nil
}

func importWebRTC(token string) error {
	hassAPI, err := hass.NewAPI("ws://supervisor/core/websocket", token)
	if err != nil {
		return err
	}

	webrtcEntities, err := hassAPI.GetWebRTCEntities()
	if err != nil {
		return err
	}

	if len(webrtcEntities) == 0 {
		log.Debug().Msg("[hass] webrtc cameras not found")
	}

	for name, entityID := range webrtcEntities {
		entities[name] = "hass://supervisor?entity_id=" + entityID

		log.Debug().Msgf("[hass] load webrtc name=%s entity_id=%s", name, entityID)
	}

	return nil
}

var entities = map[string]string{}
var log zerolog.Logger
var once sync.Once
