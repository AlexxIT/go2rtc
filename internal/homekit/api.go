package homekit

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app/store"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

func apiHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		items := make([]any, 0)

		for name, src := range store.GetDict("streams") {
			if src := src.(string); strings.HasPrefix(src, "homekit") {
				u, err := url.Parse(src)
				if err != nil {
					continue
				}
				device := Device{
					Name:   name,
					Addr:   u.Host,
					Paired: true,
				}
				items = append(items, device)
			}
		}

		err := mdns.Discovery(mdns.ServiceHAP, func(entry *mdns.ServiceEntry) bool {
			log.Trace().Msgf("[homekit] mdns=%s", entry)

			if entry.Complete() {
				device := Device{
					Name:   entry.Name,
					Addr:   entry.Addr(),
					ID:     entry.Info[hap.TXTDeviceID],
					Model:  entry.Info[hap.TXTModel],
					Paired: entry.Info[hap.TXTStatusFlags] == "0",
				}
				items = append(items, device)
			}
			return false
		})

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		api.ResponseJSON(w, items)

	case "POST":
		// TODO: post params...

		id := r.URL.Query().Get("id")
		pin := r.URL.Query().Get("pin")
		name := r.URL.Query().Get("name")
		if err := hkPair(id, pin, name); err != nil {
			log.Error().Err(err).Caller().Send()
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "DELETE":
		src := r.URL.Query().Get("src")
		if err := hkDelete(src); err != nil {
			log.Error().Err(err).Caller().Send()
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func hkPair(deviceID, pin, name string) (err error) {
	var conn *hap.Client

	if conn, err = hap.Pair(deviceID, pin); err != nil {
		return
	}

	streams.New(name, conn.URL())

	dict := store.GetDict("streams")
	dict[name] = conn.URL()

	return store.Set("streams", dict)
}

func hkDelete(name string) (err error) {
	dict := store.GetDict("streams")
	for key, rawURL := range dict {
		if key != name {
			continue
		}

		var conn *hap.Client

		if conn, err = hap.NewClient(rawURL.(string)); err != nil {
			return
		}

		if err = conn.Dial(); err != nil {
			return
		}

		if err = conn.ListPairings(); err != nil {
			return
		}

		if err = conn.DeletePairing(conn.ClientID); err != nil {
			log.Error().Err(err).Caller().Send()
		}

		delete(dict, name)

		return store.Set("streams", dict)
	}

	return nil
}

type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Addr   string `json:"addr"`
	Model  string `json:"model"`
	Paired bool   `json:"paired"`
	//Type    string `json:"type"`
}
