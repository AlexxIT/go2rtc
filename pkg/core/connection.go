package core

import (
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
)

func NewID() uint32 {
	return id.Add(1)
}

// Deprecated: use NewID instead
func ID(v any) uint32 {
	p := uintptr(reflect.ValueOf(v).UnsafePointer())
	return 0x8000_0000 | uint32(p)
}

var id atomic.Uint32

type Info interface {
	SetProtocol(string)
	SetRemoteAddr(string)
	SetSource(string)
	SetURL(string)
	WithRequest(*http.Request)
}

// Connection just like webrtc.PeerConnection
// - ID and RemoteAddr used for building Connection(s) graph
// - FormatName, Protocol, RemoteAddr, Source, URL, SDP, UserAgent used for info about Connection
// - FormatName and Protocol has FFmpeg compatible names
// - Transport used for auto closing on Stop
type Connection struct {
	ID         uint32 `json:"id,omitempty"`
	FormatName string `json:"format_name,omitempty"` // rtsp, webrtc, mp4, mjpeg, mpjpeg...
	Protocol   string `json:"protocol,omitempty"`    // tcp, udp, http, ws, pipe...
	RemoteAddr string `json:"remote_addr,omitempty"` // host:port other info
	Source     string `json:"source,omitempty"`
	URL        string `json:"url,omitempty"`
	SDP        string `json:"sdp,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`

	Medias    []*Media    `json:"medias,omitempty"`
	Receivers []*Receiver `json:"receivers,omitempty"`
	Senders   []*Sender   `json:"senders,omitempty"`
	Recv      int         `json:"bytes_recv,omitempty"`
	Send      int         `json:"bytes_send,omitempty"`

	Transport any `json:"-"`
}

func (c *Connection) GetMedias() []*Media {
	return c.Medias
}

func (c *Connection) GetTrack(media *Media, codec *Codec) (*Receiver, error) {
	for _, receiver := range c.Receivers {
		if receiver.Codec == codec {
			return receiver, nil
		}
	}
	receiver := NewReceiver(media, codec)
	c.Receivers = append(c.Receivers, receiver)
	return receiver, nil
}

func (c *Connection) Stop() error {
	for _, receiver := range c.Receivers {
		receiver.Close()
	}
	for _, sender := range c.Senders {
		sender.Close()
	}
	if closer, ok := c.Transport.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Deprecated:
func (c *Connection) Codecs() []*Codec {
	codecs := make([]*Codec, len(c.Senders))
	for i, sender := range c.Senders {
		codecs[i] = sender.Codec
	}
	return codecs
}

func (c *Connection) SetProtocol(s string) {
	c.Protocol = s
}

func (c *Connection) SetRemoteAddr(s string) {
	if c.RemoteAddr == "" {
		c.RemoteAddr = s
	} else {
		c.RemoteAddr += " forwarded " + s
	}
}

func (c *Connection) SetSource(s string) {
	c.Source = s
}

func (c *Connection) SetURL(s string) {
	c.URL = s
}

func (c *Connection) WithRequest(r *http.Request) {
	if r.Header.Get("Upgrade") == "websocket" {
		c.Protocol = "ws"
	} else {
		c.Protocol = "http"
	}

	c.RemoteAddr = r.RemoteAddr
	if remote := r.Header.Get("X-Forwarded-For"); remote != "" {
		c.RemoteAddr += " forwarded " + remote
	}

	c.UserAgent = r.UserAgent()
}

// Create like os.Create, init Consumer with existing Transport
func Create(w io.Writer) (*Connection, error) {
	return &Connection{Transport: w}, nil
}

// Open like os.Open, init Producer from existing Transport
func Open(r io.Reader) (*Connection, error) {
	return &Connection{Transport: r}, nil
}

// Dial like net.Dial, init Producer via Dialing
func Dial(rawURL string) (*Connection, error) {
	return &Connection{}, nil
}
