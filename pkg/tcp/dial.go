package tcp

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

// Dial - for RTSP(S|X) and RTMP(S|X)
func Dial(u *url.URL, timeout time.Duration) (net.Conn, error) {
	return DialContext(context.Background(), u, timeout)
}

// DialContext is Dial with a context: cancelling ctx aborts the TCP connect
// and the TLS handshake at once, the timeout still applies to both.
func DialContext(ctx context.Context, u *url.URL, timeout time.Duration) (net.Conn, error) {
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

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	if secure == nil {
		return conn, nil
	}

	// bound the handshake by the same timeout as the TCP connect,
	// otherwise a peer that accepts but never speaks TLS blocks forever
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	tlsConn := tls.Client(conn, secure)
	if err = tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	if u.Scheme[4] == 'x' {
		u.Scheme = u.Scheme[:4] + "s"
	}

	return tlsConn, nil
}
