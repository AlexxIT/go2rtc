package tcp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// Dial - for RTSP(S|X) and RTMP(S|X)
func Dial(u *url.URL, timeout time.Duration) (net.Conn, error) {
	var address string
	var hostname string // without port
	if i := strings.IndexByte(u.Host, ':'); i > 0 {
		address = u.Host
		hostname = u.Host[:i]
	} else {
		switch u.Scheme {
		case "rtsp", "rtsps", "rtspx":
			address = u.Host + ":554"
		case "rtmp":
			address = u.Host + ":1935"
		case "rtmps", "rtmpx":
			address = u.Host + ":443"
		}
		hostname = u.Host
	}

	var secure *tls.Config

	switch u.Scheme {
	case "rtsp", "rtmp":
	case "rtsps", "rtspx", "rtmps", "rtmpx":
		if u.Scheme[4] == 'x' || IsIP(hostname) {
			secure = &tls.Config{InsecureSkipVerify: true}
		} else {
			secure = &tls.Config{ServerName: hostname}
		}
	default:
		return nil, errors.New("unsupported scheme: " + u.Scheme)
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}

	if secure == nil {
		return conn, nil
	}

	tlsConn := tls.Client(conn, secure)
	if err = tlsConn.Handshake(); err != nil {
		return nil, err
	}

	if u.Scheme[4] == 'x' {
		u.Scheme = u.Scheme[:4] + "s"
	}

	return tlsConn, nil
}
