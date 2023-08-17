package rtmp

import (
	"io"
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/rtmp"
	"github.com/rs/zerolog/log"
)

func Init() {
	streams.HandleFunc("rtmp", streamsHandle)
	streams.HandleFunc("rtmps", streamsHandle)
	streams.HandleFunc("rtmpx", streamsHandle)

	api.HandleFunc("api/stream.flv", apiHandle)
}

func streamsHandle(url string) (core.Producer, error) {
	client, err := rtmp.Dial(url)
	if err != nil {
		return nil, err
	}
	return client, nil
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

	client, err := flv.Open(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.AddProducer(client)

	if err = client.Start(); err != nil && err != io.EOF {
		log.Warn().Err(err).Caller().Send()
	}

	stream.RemoveProducer(client)
}
