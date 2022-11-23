package api

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
)

func initWS(origin string) {
	wsUp = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 512000,
	}

	if origin == "*" {
		wsUp.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}
}

var wsUp *websocket.Upgrader

type WSHandler func(ctx *Context, msg *streamer.Message)

type Context struct {
	Conn     *websocket.Conn
	Request  *http.Request
	Consumer interface{} // TODO: rewrite

	onClose []func()
	mu      sync.Mutex
}

func (ctx *Context) Upgrade(w http.ResponseWriter, r *http.Request) (err error) {
	ctx.Conn, err = wsUp.Upgrade(w, r, nil)
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

	if data, ok := msg.([]byte); ok {
		_ = ctx.Conn.WriteMessage(websocket.BinaryMessage, data)
	} else {
		_ = ctx.Conn.WriteJSON(msg)
	}

	ctx.mu.Unlock()
}

func (ctx *Context) Error(err error) {
	ctx.Write(&streamer.Message{
		Type: "error", Value: err.Error(),
	})
}

func (ctx *Context) OnClose(f func()) {
	ctx.onClose = append(ctx.onClose, f)
}
