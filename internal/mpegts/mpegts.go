package mpegts

import (
	"net/http"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/rs/zerolog/log"
)

func Init() {
	api.HandleFunc("api/stream.ts", apiHandle)
	api.HandleFunc("api/stream.aac", apiStreamAAC)
}

func apiHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		outputMpegTS(w, r)
	} else {
		inputMpegTS(w, r)
	}
}

func outputMpegTS(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	cons := mpegts.NewConsumer()
	cons.RemoteAddr = tcp.RemoteAddr(r)
	cons.UserAgent = r.UserAgent()

	if err := stream.AddConsumer(cons); err != nil {
		log.Error().Err(err).Caller().Send()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add("Content-Type", "video/mp2t")

	_, _ = cons.WriteTo(w)

	stream.RemoveConsumer(cons)
}

func inputMpegTS(w http.ResponseWriter, r *http.Request) {
	dst := r.URL.Query().Get("dst")
	stream := streams.Get(dst)
	if stream == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}

	res := &http.Response{Body: r.Body, Request: r}
	client, err := mpegts.Open(res.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.AddProducer(client)

	if err = client.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	stream.RemoveProducer(client)
}
