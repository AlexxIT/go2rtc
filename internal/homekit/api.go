package homekit

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"strconv"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

func apiHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		sources, err := discovery()
		if err != nil {
			api.Error(w, err)
			return
		}

		urls := findHomeKitURLs()
		for id, u := range urls {
			deviceID := u.Query().Get("device_id")
			for _, source := range sources {
				if strings.Contains(source.URL, deviceID) {
					source.Location = id
					break
				}
			}
		}

		for _, source := range sources {
			if source.Location == "" {
				source.Location = " "
			}
		}

		api.ResponseSources(w, sources)

	case "POST":
		if err := r.ParseMultipartForm(1024); err != nil {
			api.Error(w, err)
			return
		}

		if err := apiPair(r.Form.Get("id"), r.Form.Get("url")); err != nil {
			api.Error(w, err)
		}

	case "DELETE":
		if err := r.ParseMultipartForm(1024); err != nil {
			api.Error(w, err)
			return
		}

		if err := apiUnpair(r.Form.Get("id")); err != nil {
			api.Error(w, err)
		}
	}
}

func discovery() ([]*api.Source, error) {
	var sources []*api.Source

	// 1. Get streams from Discovery
	err := mdns.Discovery(mdns.ServiceHAP, func(entry *mdns.ServiceEntry) bool {
		log.Trace().Msgf("[homekit] mdns=%s", entry)

		category := entry.Info[hap.TXTCategory]
		if entry.Complete() && (category == hap.CategoryCamera || category == hap.CategoryDoorbell) {
			source := &api.Source{
				Name: entry.Name,
				Info: entry.Info[hap.TXTModel],
				URL: fmt.Sprintf(
					"homekit://%s:%d?device_id=%s&feature=%s&status=%s",
					entry.IP, entry.Port, entry.Info[hap.TXTDeviceID],
					entry.Info[hap.TXTFeatureFlags], entry.Info[hap.TXTStatusFlags],
				),
			}

			sources = append(sources, source)
		}
		return false
	})

	if err != nil {
		return nil, err
	}

	return sources, nil
}

func apiPair(id, url string) error {
	conn, err := hap.Pair(url)
	if err != nil {
		return err
	}

	streams.New(id, conn.URL())

	return app.PatchConfig(id, conn.URL(), "streams")
}

func apiUnpair(id string) error {
	stream := streams.Get(id)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	rawURL := findHomeKitURL(stream)
	if rawURL == "" {
		return errors.New("not homekit source")
	}

	if err := hap.Unpair(rawURL); err != nil {
		return err
	}

	streams.Delete(id)

	return app.PatchConfig(id, nil, "streams")
}

func findHomeKitURLs() map[string]*url.URL {
	urls := map[string]*url.URL{}
	for id, stream := range streams.Streams() {
		if rawURL := findHomeKitURL(stream); rawURL != "" {
			if u, err := url.Parse(rawURL); err == nil {
				urls[id] = u
			}
		}
	}
	return urls
}

type PairingInfo struct {
	Name         string     `yaml:"name"`
	DeviceID     string     `yaml:"device_id"`
	Pin          string     `yaml:"pin"`
	Status       string     `yaml:"status"`
	SetupURI     string     `yaml:"setup_uri"`
}

func getPairingInfo(host string, s *server) PairingInfo {
	// for QR-Code
	category, _ := strconv.ParseInt(hap.CategoryCamera, 10, 64)
	pin, _ := strconv.ParseInt(strings.Replace(s.hap.Pin, "-", "", -1), 10, 64)
	payload := "00000000" + strconv.FormatInt(category << 31 + 1 << 28 + pin, 36)
	uri := strings.ToUpper("X-HM://" + payload[len(payload)-9:] + s.hap.SetupID[:4])
	status := "unpaired"
	if len(s.pairings) > 0 {
		status = "paired"
	}
	return PairingInfo {
		Name: s.mdns.Name ,
		DeviceID: s.hap.DeviceID,
		SetupURI: uri,
		Pin: s.hap.Pin,
		Status: status,
	}
}

func apiPairingHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		pairingInfo := map[string]PairingInfo{}
		for host, s := range servers {
			pairingInfo[s.stream] = getPairingInfo(host, s)
		}
		api.ResponseJSON(w, pairingInfo)

	case "DELETE":
		query := r.URL.Query()
		name := query.Get("name")
		stream := query.Get("stream")
		device_id := query.Get("device_id")
		for _, s := range servers {
			if name == s.mdns.Name || stream == s.stream || device_id == s.hap.DeviceID {
				s.pairings = nil
				s.UpdateStatus()
				s.PatchConfig()
				break;
			}
		}
		discovery()
	}
}
