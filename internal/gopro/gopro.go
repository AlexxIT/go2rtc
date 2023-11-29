package gopro

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/gopro"
)

func Init() {
	streams.HandleFunc("gopro", handleGoPro)

	api.HandleFunc("api/gopro", apiGoPro)
}

func handleGoPro(rawURL string) (core.Producer, error) {
	return gopro.Dial(rawURL)
}

func apiGoPro(w http.ResponseWriter, r *http.Request) {
	var items []*api.Source

	for _, host := range gopro.Discovery() {
		items = append(items, &api.Source{Name: host, URL: "gopro://" + host})
	}

	api.ResponseSources(w, items)
}
