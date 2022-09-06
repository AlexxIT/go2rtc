package api

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

func initWS() {
	wsUp = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 512000,
	}
	wsUp.CheckOrigin = func(r *http.Request) bool {
		origin := r.Header["Origin"]
		if len(origin) == 0 {
			return true
		}
		o, err := url.Parse(origin[0])
		if err != nil {
			return false
		}
		if o.Host == r.Host {
			return true
		}
		log.Trace().Msgf("[api.ws] origin: %s, host: %s", o.Host, r.Host)
		// some users change Nginx external port using Docker port
		// so origin will be with a port and host without
		if i := strings.IndexByte(o.Host, ':'); i > 0 {
			return o.Host[:i] == r.Host
		}
		return false
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
