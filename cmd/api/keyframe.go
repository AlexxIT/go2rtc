package api

import (
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/keyframe"
	"net/http"
	"strings"
)

func frameHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	stream := streams.Get(url)
	if stream == nil {
		return
	}

	var ch = make(chan []byte)

	cons := new(keyframe.Consumer)
	cons.IsMP4 = strings.HasSuffix(r.URL.Path, ".mp4")
	cons.Listen(func(msg interface{}) {
		switch msg.(type) {
		case []byte:
			ch <- msg.([]byte)
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Warn().Err(err).Msg("[api.frame] add consumer")
		return
	}

	data := <-ch

	stream.RemoveConsumer(cons)

	if _, err := w.Write(data); err != nil {
		log.Error().Err(err).Msg("[api.frame] write")
	}
}
