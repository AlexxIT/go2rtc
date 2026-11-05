package baichuan

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultPort    = 9000
	DefaultTimeout = 10 * time.Second
)

const (
	defaultMaxBody        = 16 << 20
	defaultMaxExtension   = 64 << 10
	defaultMaxPending     = 64
	defaultMaxMediaFrame  = 16 << 20
	defaultMaxResync      = 64 << 10
	defaultMaxMediaBuffer = defaultMaxMediaFrame + defaultMaxResync + mediaVideoHeaderSize
)

// Limits bounds memory controlled by a camera or slow peer.
type Limits struct {
	MaxBody        uint32
	MaxExtension   uint32
	MaxPending     int
	MaxMediaFrame  uint32
	MaxMediaBuffer uint32
	MaxResync      uint32
}

func (l Limits) normalized() (Limits, error) {
	if l.MaxBody == 0 {
		l.MaxBody = defaultMaxBody
	}
	if l.MaxExtension == 0 {
		l.MaxExtension = defaultMaxExtension
	}
	if l.MaxPending == 0 {
		l.MaxPending = defaultMaxPending
	}
	if l.MaxMediaFrame == 0 {
		l.MaxMediaFrame = defaultMaxMediaFrame
	}
	if l.MaxMediaBuffer == 0 {
		l.MaxMediaBuffer = defaultMaxMediaBuffer
	}
	if l.MaxResync == 0 {
		l.MaxResync = defaultMaxResync
	}

	if l.MaxBody > 64<<20 || l.MaxExtension > 1<<20 || l.MaxPending > 1024 ||
		l.MaxMediaFrame > 64<<20 || l.MaxMediaBuffer > 128<<20 || l.MaxResync > 1<<20 {
		return Limits{}, fmt.Errorf("baichuan: limits exceed hard ceiling")
	}
	if l.MaxPending < 1 || l.MaxExtension > l.MaxBody || l.MaxResync > l.MaxMediaBuffer ||
		uint64(l.MaxMediaFrame)+uint64(l.MaxResync)+mediaVideoHeaderSize > uint64(l.MaxMediaBuffer) {
		return Limits{}, fmt.Errorf("baichuan: inconsistent limits")
	}
	return l, nil
}

// Config contains a private camera endpoint and credentials.
type Config struct {
	Host    string
	Port    uint16
	Timeout time.Duration
	Limits  Limits
	// UIDLocalAddr restricts UID discovery to the interface owning this IPv4 address.
	UIDLocalAddr string
	// UIDBroadcastAddr overrides the destination used for UID discovery.
	UIDBroadcastAddr string

	username string
	password string
	uid      string
}

func NewConfig(host, username, password string) Config {
	return Config{Host: host, username: username, password: password}
}

func NewUIDConfig(uid, username, password string) Config {
	return Config{uid: uid, username: username, password: password}
}

func (c Config) normalized() (Config, error) {
	if c.uid != "" {
		if c.Host != "" || c.Port != 0 || !validUID(c.uid) {
			return Config{}, fmt.Errorf("baichuan: invalid UID endpoint")
		}
		if c.UIDLocalAddr != "" {
			address, err := netip.ParseAddr(strings.TrimSpace(c.UIDLocalAddr))
			if err != nil || !uidUnicastAddr(address) {
				return Config{}, fmt.Errorf("baichuan: invalid UID local address")
			}
			c.UIDLocalAddr = address.String()
		}
		if c.UIDBroadcastAddr != "" {
			address, err := netip.ParseAddr(strings.TrimSpace(c.UIDBroadcastAddr))
			if err != nil || !uidDiscoveryAddr(address) {
				return Config{}, fmt.Errorf("baichuan: invalid UID broadcast address")
			}
			c.UIDBroadcastAddr = address.String()
		}
	} else {
		if c.UIDLocalAddr != "" {
			return Config{}, fmt.Errorf("baichuan: UID local address requires UID transport")
		}
		if c.UIDBroadcastAddr != "" {
			return Config{}, fmt.Errorf("baichuan: UID broadcast address requires UID transport")
		}
		c.Host = strings.TrimSpace(c.Host)
		if c.Host == "" {
			return Config{}, fmt.Errorf("baichuan: host is required")
		}
		if strings.ContainsAny(c.Host, "/?#@") {
			return Config{}, fmt.Errorf("baichuan: host must not contain URL components")
		}
		if c.Port == 0 {
			c.Port = DefaultPort
		}
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Timeout < 0 {
		return Config{}, fmt.Errorf("baichuan: timeout must be positive")
	}

	var err error
	if c.Limits, err = c.Limits.normalized(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) address() string {
	return net.JoinHostPort(c.Host, strconv.Itoa(int(c.Port)))
}

func (c Config) Format(s fmt.State, _ rune) {
	if c.uid != "" {
		_, _ = fmt.Fprintf(s, "baichuan.Config{Mode:%q, LocalAddr:%q, BroadcastAddr:%q, Timeout:%s}",
			"uid", c.UIDLocalAddr, c.UIDBroadcastAddr, c.Timeout)
		return
	}
	_, _ = fmt.Fprintf(s, "baichuan.Config{Host:%q, Port:%d, Timeout:%s}", c.Host, c.Port, c.Timeout)
}

func (c Config) MarshalJSON() ([]byte, error) {
	if c.uid != "" {
		return json.Marshal(struct {
			Mode          string        `json:"mode"`
			LocalAddr     string        `json:"local_addr,omitempty"`
			BroadcastAddr string        `json:"broadcast_addr,omitempty"`
			Timeout       time.Duration `json:"timeout"`
		}{"uid", c.UIDLocalAddr, c.UIDBroadcastAddr, c.Timeout})
	}
	return json.Marshal(struct {
		Host    string        `json:"host"`
		Port    uint16        `json:"port"`
		Timeout time.Duration `json:"timeout"`
	}{c.Host, c.Port, c.Timeout})
}

// Stream selects one preview profile exposed by the camera.
type Stream string

const (
	StreamMain   Stream = "mainStream"
	StreamSub    Stream = "subStream"
	StreamExtern Stream = "externStream"
)

func (s Stream) params() (uint8, uint32, error) {
	switch s {
	case StreamMain:
		return 0, 0, nil
	case StreamSub:
		return 1, 256, nil
	case StreamExtern:
		return 2, 1024, nil
	default:
		return 0, 0, fmt.Errorf("baichuan: unsupported stream %q", s)
	}
}
