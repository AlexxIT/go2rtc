package mp4

import (
	"github.com/AlexxIT/go2rtc/cmd/api"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

const MsgTypeMSE = "mse" // fMP4

func handlerWS(ctx *api.Context, msg *streamer.Message) {
	src := ctx.Request.URL.Query().Get("src")
	stream := streams.GetOrNew(src)
	if stream == nil {
		return
	}

	cons := &mp4.Consumer{}
	cons.UserAgent = ctx.Request.UserAgent()
	cons.RemoteAddr = ctx.Request.RemoteAddr

	cons.Listen(func(msg interface{}) {
		switch msg.(type) {
		case *streamer.Message, []byte:
			ctx.Write(msg)
		}
	})

	if err := stream.AddConsumer(cons); err != nil {
		log.Warn().Err(err).Msg("[api.mse] add consumer")
		ctx.Error(err)
		return
	}

	ctx.OnClose(func() {
		stream.RemoveConsumer(cons)
	})

	ctx.Write(&streamer.Message{
		Type: MsgTypeMSE, Value: cons.MimeType(),
	})

	data, err := cons.Init()
	if err != nil {
		log.Warn().Err(err).Msg("[api.mse] init")
		ctx.Error(err)
		return
	}

	ctx.Write(data)
}
