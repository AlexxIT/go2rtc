package api

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
)

type WSHandler func(ctx *Context, msg *streamer.Message)

var apiWsUp = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 512000,
}

type Context struct {
	Conn     *websocket.Conn
	Request  *http.Request
	Consumer interface{} // TODO: rewrite

	onClose []func()
	mu      sync.Mutex
}

func (ctx *Context) Upgrade(w http.ResponseWriter, r *http.Request) (err error) {
	ctx.Conn, err = apiWsUp.Upgrade(w, r, nil)
	ctx.Request = r
	return
}

func (ctx *Context) Close() {
	for _, f := range ctx.onClose {
		f()
	}
	_ = ctx.Conn.Close()
}

func (ctx *Context) Write(msg interface{}) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	var err error

	switch msg := msg.(type) {
	case *streamer.Message:
		err = ctx.Conn.WriteJSON(msg)
	case []byte:
		err = ctx.Conn.WriteMessage(websocket.BinaryMessage, msg)
	default:
		return
	}

	if err != nil {
		//panic(err) // TODO: fix panic
	}
}

func (ctx *Context) Error(err error) {
	ctx.Write(&streamer.Message{
		Type: "error", Value: err.Error(),
	})
}

func (ctx *Context) OnClose(f func()) {
	ctx.onClose = append(ctx.onClose, f)
}
