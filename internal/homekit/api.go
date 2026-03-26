package homekit

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
)

func apiDiscovery(w http.ResponseWriter, r *http.Request) {
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
}

func apiHomekit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		if id := r.Form.Get("id"); id != "" {
			if srv := servers[id]; srv != nil {
				api.ResponsePrettyJSON(w, srv)
			} else {
				http.Error(w, "server not found", http.StatusNotFound)
			}
		} else {
			api.ResponsePrettyJSON(w, servers)
		}

	case "POST":
		id := r.Form.Get("id")
		rawURL := r.Form.Get("src") + "&pin=" + r.Form.Get("pin")
		if err := apiPair(id, rawURL); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "DELETE":
		id := r.Form.Get("id")
		if err := apiUnpair(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func apiHomekitAccessories(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	stream := streams.Get(id)
	if stream == nil {
		http.Error(w, "", http.StatusNotFound)
		return
	}

	rawURL := findHomeKitURL(stream.Sources())
	if rawURL == "" {
		http.Error(w, "", http.StatusBadRequest)
		return
	}

	client, err := hap.Dial(rawURL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer client.Close()

	res, err := client.Get(hap.PathAccessories)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", api.MimeJSON)
	_, _ = io.Copy(w, res.Body)
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

	return app.PatchConfig([]string{"streams", id}, conn.URL())
}

func apiUnpair(id string) error {
	stream := streams.Get(id)
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	rawURL := findHomeKitURL(stream.Sources())
	if rawURL == "" {
		return errors.New("not homekit source")
	}

	if err := hap.Unpair(rawURL); err != nil {
		return err
	}

	streams.Delete(id)

	return app.PatchConfig([]string{"streams", id}, nil)
}

func apiHomekitEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher.Flush()

	ch := make(chan DoorbellEvent, 8)
	addSSEListener(ch)
	defer removeSSEListener(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: doorbell\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func findHomeKitURLs() map[string]*url.URL {
	urls := map[string]*url.URL{}
	for name, sources := range streams.GetAllSources() {
		if rawURL := findHomeKitURL(sources); rawURL != "" {
			if u, err := url.Parse(rawURL); err == nil {
				urls[name] = u
			}
		}
	}
	return urls
}
