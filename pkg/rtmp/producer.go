package rtmp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
)

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	var hostname string // without port
	if i := strings.IndexByte(u.Host, ':'); i > 0 {
		hostname = u.Host[:i]
	} else {
		hostname = u.Host
		u.Host += ":1935"
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "rtmp" {
		var conf *tls.Config

		switch {
		case u.Scheme == "rtmpx" || net.ParseIP(hostname) != nil:
			conf = &tls.Config{InsecureSkipVerify: true}
		case u.Scheme == "rtmps":
			conf = &tls.Config{ServerName: hostname}
		default:
			return nil, errors.New("unsupported scheme: " + u.Scheme)
		}

		tlsConn := tls.Client(conn, conf)
		if err = tlsConn.Handshake(); err != nil {
			return nil, err
		}
		conn = tlsConn
	}

	rd, err := NewReader(u, conn)
	if err != nil {
		return nil, err
	}

	return flv.Open(rd)
}
