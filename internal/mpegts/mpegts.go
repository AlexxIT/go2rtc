package mpegts

import (
	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"net/http"
)

func Init() {
	api.HandleFunc("api/stream.ts", apiHandle)
}

func apiHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}

	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	res := &http.Response{Body: r.Body, Request: r}
	client := mpegts.NewClient(res)

	if err := client.Handle(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.AddProducer(client)

	if err := client.Handle(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.RemoveProducer(client)
}
