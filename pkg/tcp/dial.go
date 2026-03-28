package tcp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"
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

	var conn net.Conn
	var err error

	if proxyAddr := fragmentParam(u.Fragment, "proxy"); proxyAddr != "" {
		conn, err = dialSocks5(address, proxyAddr, timeout)
	} else {
		conn, err = net.DialTimeout("tcp", address, timeout)
	}
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

// fragmentParam extracts a named parameter from a go2rtc-style fragment string.
// Fragment parameters are separated by "#" and formatted as "key=value".
func fragmentParam(fragment, key string) string {
	prefix := key + "="
	for _, part := range strings.Split(fragment, "#") {
		if strings.HasPrefix(part, prefix) {
			return part[len(prefix):]
		}
	}
	return ""
}

// dialSocks5 connects to address through a SOCKS5 proxy.
// proxyURL format: socks5://[user:pass@]host:port
func dialSocks5(address, proxyURL string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	var auth *proxy.Auth
	if u.User != nil {
		auth = &proxy.Auth{
			User: u.User.Username(),
		}
		auth.Password, _ = u.User.Password()
	}

	dialer, err := proxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: timeout})
	if err != nil {
		return nil, err
	}

	return dialer.Dial("tcp", address)
}
