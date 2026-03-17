package webcodecs

import (
	"errors"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/api/ws"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/webcodecs"
	"github.com/rs/zerolog"
)

func Init() {
	log = app.GetLogger("webcodecs")

	ws.HandleFunc("webcodecs", handlerWS)
}

var log zerolog.Logger

func handlerWS(tr *ws.Transport, msg *ws.Message) error {
	stream, _ := streams.GetOrPatch(tr.Request.URL.Query())
	if stream == nil {
		return errors.New(api.StreamNotFound)
	}

	cons := webcodecs.NewConsumer(nil)
	cons.WithRequest(tr.Request)

	if err := stream.AddConsumer(cons); err != nil {
		log.Debug().Err(err).Msg("[webcodecs] add consumer")
		return err
	}

	tr.Write(&ws.Message{Type: "webcodecs", Value: cons.GetInitInfo()})

	go cons.WriteTo(tr.Writer())

	tr.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	return nil
}
