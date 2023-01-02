package api

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Message - struct for data exchange in Web API
type Message struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value,omitempty"`
}

type WSHandler func(tr *Transport, msg *Message) error

func HandleWS(msgType string, handler WSHandler) {
	wsHandlers[msgType] = handler
}

var wsHandlers = make(map[string]WSHandler)

func initWS(origin string) {
	wsUp = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 2028,
	}

	switch origin {
	case "":
		// same origin + ignore port
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
			log.Trace().Msgf("[api.ws] origin=%s, host=%s", o.Host, r.Host)
			// https://github.com/AlexxIT/go2rtc/issues/118
			if i := strings.IndexByte(o.Host, ':'); i > 0 {
				return o.Host[:i] == r.Host
			}
			return false
		}
	case "*":
		// any origin
		wsUp.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}
}

func apiWS(w http.ResponseWriter, r *http.Request) {
	ws, err := wsUp.Upgrade(w, r, nil)
	if err != nil {
		origin := r.Header.Get("Origin")
		log.Error().Err(err).Caller().Msgf("host=%s origin=%s", r.Host, origin)
		return
	}

	tr := &Transport{Request: r}
	tr.OnWrite(func(msg interface{}) {
		if data, ok := msg.([]byte); ok {
			_ = ws.WriteMessage(websocket.BinaryMessage, data)
		} else {
			_ = ws.WriteJSON(msg)
		}
	})

	for {
		msg := new(Message)
		if err = ws.ReadJSON(msg); err != nil {
			log.Trace().Err(err).Caller().Send()
			_ = ws.Close()
			break
		}

		if handler := wsHandlers[msg.Type]; handler != nil {
			go func() {
				if err = handler(tr, msg); err != nil {
					tr.Write(&Message{Type: "error", Value: msg.Type + ": " + err.Error()})
				}
			}()
		}
	}

	tr.Close()
}

var wsUp *websocket.Upgrader

type Transport struct {
	Request  *http.Request
	Consumer interface{} // TODO: rewrite

	mx sync.Mutex

	onChange func()
	onWrite  func(msg interface{})
	onClose  []func()
}

func (t *Transport) OnWrite(f func(msg interface{})) {
	t.mx.Lock()
	if t.onChange != nil {
		t.onChange()
	}
	t.onWrite = f
	t.mx.Unlock()
}

func (t *Transport) Write(msg interface{}) {
	t.mx.Lock()
	t.onWrite(msg)
	t.mx.Unlock()
}

func (t *Transport) Close() {
	for _, f := range t.onClose {
		f()
	}
}

func (t *Transport) OnChange(f func()) {
	t.onChange = f
}

func (t *Transport) OnClose(f func()) {
	t.onClose = append(t.onClose, f)
}
