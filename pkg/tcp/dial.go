package tcp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// Dial - for RTSP(S|X) and RTMP(S|X)
func Dial(u *url.URL, port string) (net.Conn, error) {
	var hostname string // without port
	if i := strings.IndexByte(u.Host, ':'); i > 0 {
		hostname = u.Host[:i]
	} else {
		hostname = u.Host
		u.Host += ":" + port
	}

	var secure *tls.Config

	switch u.Scheme {
	case "rtsp", "rtmp":
	case "rtsps", "rtspx", "rtmps", "rtmpx":
		if u.Scheme[4] == 'x' || net.ParseIP(hostname) != nil {
			secure = &tls.Config{InsecureSkipVerify: true}
		} else {
			secure = &tls.Config{ServerName: hostname}
		}
	default:
		return nil, errors.New("unsupported scheme: " + u.Scheme)
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
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
