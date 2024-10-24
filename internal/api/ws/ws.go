package ws

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

func Init() {
	var cfg struct {
		Mod struct {
			Origin string `yaml:"origin"`
		} `yaml:"api"`
	}

	app.LoadConfig(&cfg)

	log = app.GetLogger("api")

	initWS(cfg.Mod.Origin)

	api.HandleFunc("api/ws", apiWS)
}

var log zerolog.Logger

// Message - struct for data exchange in Web API
type Message struct {
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
	Raw   []byte `json:"-"`
}

func (m *Message) String() (value string) {
	_ = json.Unmarshal(m.Raw, &value)
	return
}

func (m *Message) Unmarshal(v any) error {
	return json.Unmarshal(m.Raw, v)
}

type WSHandler func(tr *Transport, msg *Message) error

func HandleFunc(msgType string, handler WSHandler) {
	wsHandlers[msgType] = handler
}

var wsHandlers = make(map[string]WSHandler)

func initWS(origin string) {
	wsUp = &websocket.Upgrader{
		ReadBufferSize:  4096,       // for SDP
		WriteBufferSize: 512 * 1024, // 512K
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
			log.Trace().Msgf("[api] ws origin=%s, host=%s", o.Host, r.Host)
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
	tr.OnWrite(func(msg any) error {
		_ = ws.SetWriteDeadline(time.Now().Add(time.Second * 5))

		if data, ok := msg.([]byte); ok {
			return ws.WriteMessage(websocket.BinaryMessage, data)
		} else {
			return ws.WriteJSON(msg)
		}
	})

	for {
		var raw struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		}
		if err = ws.ReadJSON(&raw); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNoStatusReceived) {
				log.Trace().Err(err).Caller().Send()
			}
			_ = ws.Close()
			break
		}

		msg := &Message{Type: raw.Type, Raw: raw.Value}

		log.Trace().Str("type", msg.Type).Msg("[api] ws msg")

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
	Request *http.Request

	ctx map[any]any

	closed bool
	mx     sync.Mutex
	wrmx   sync.Mutex

	onChange func()
	onWrite  func(msg any) error
	onClose  []func()
}

func (t *Transport) OnWrite(f func(msg any) error) {
	t.mx.Lock()
	if t.onChange != nil {
		t.onChange()
	}
	t.onWrite = f
	t.mx.Unlock()
}

func (t *Transport) Write(msg any) {
	t.wrmx.Lock()
	_ = t.onWrite(msg)
	t.wrmx.Unlock()
}

func (t *Transport) Close() {
	t.mx.Lock()
	for _, f := range t.onClose {
		f()
	}
	t.closed = true
	t.mx.Unlock()
}

func (t *Transport) OnChange(f func()) {
	t.mx.Lock()
	t.onChange = f
	t.mx.Unlock()
}

func (t *Transport) OnClose(f func()) {
	t.mx.Lock()
	if t.closed {
		f()
	} else {
		t.onClose = append(t.onClose, f)
	}
	t.mx.Unlock()
}

// WithContext - run function with Context variable
func (t *Transport) WithContext(f func(ctx map[any]any)) {
	t.mx.Lock()
	if t.ctx == nil {
		t.ctx = map[any]any{}
	}
	f(t.ctx)
	t.mx.Unlock()
}

func (t *Transport) Writer() io.Writer {
	return &writer{t: t}
}

type writer struct {
	t *Transport
}

func (w *writer) Write(p []byte) (n int, err error) {
	w.t.wrmx.Lock()
	if err = w.t.onWrite(p); err == nil {
		n = len(p)
	}
	w.t.wrmx.Unlock()
	return
}
