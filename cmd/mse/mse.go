package mse

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/mse"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/rs/zerolog/log"
)

func Init() {
	api.HandleWS("mse", handler)
}

func handler(ctx *api.Context, msg *streamer.Message) {
	src := ctx.Request.URL.Query().Get("src")
	stream := streams.Get(src)
	if stream == nil {
		return
	}

	cons := new(mse.Consumer)
	cons.UserAgent = ctx.Request.UserAgent()
	cons.RemoteAddr = ctx.Request.RemoteAddr
	cons.Listen(func(msg interface{}) {
		switch msg.(type) {
		case *streamer.Message, []byte:
			ctx.Write(msg)
		}
	})
	if err := stream.AddConsumer(cons); err != nil {
		log.Warn().Err(err).Msg("[api.mse] Add consumer")
		ctx.Error(err)
		return
	}

	ctx.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	cons.Init()
}
