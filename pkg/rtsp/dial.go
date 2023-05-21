package rtsp

import (
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

func Dial(uri string) (net.Conn, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "rtsp":
		return dialTCP(u.Host, nil)
	case "rtsps":
		tlsConf := &tls.Config{ServerName: u.Hostname()}
		return dialTCP(u.Host, tlsConf)
	case "rtspx":
		tlsConf := &tls.Config{InsecureSkipVerify: true}
		return dialTCP(u.Host, tlsConf)
	}

	return nil, errors.New("unsupported scheme: " + u.Scheme)
}

func dialTCP(address string, tlsConf *tls.Config) (net.Conn, error) {
	if strings.IndexByte(address, ':') < 0 {
		address += ":554"
	}

	conn, err := net.DialTimeout("tcp", address, time.Second*5)
	if tlsConf == nil || err != nil {
		return conn, err
	}

	tlsConn := tls.Client(conn, tlsConf)
	return tlsConn, tlsConn.Handshake()
}
