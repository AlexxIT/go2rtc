package tcp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Do - http.Client with support Digest Authorization
func Do(req *http.Request) (*http.Response, error) {
	var secure *tls.Config

	switch req.URL.Scheme {
	case "httpx":
		secure = &tls.Config{InsecureSkipVerify: true}
		req.URL.Scheme = "https"
	case "https":
		if hostname := req.URL.Hostname(); IsIP(hostname) {
			secure = &tls.Config{InsecureSkipVerify: true}
		} else {
			secure = &tls.Config{ServerName: hostname}
		}
	}

	if secure != nil {
		ctx := context.WithValue(req.Context(), secureKey, secure)
		req = req.WithContext(ctx)
	}

	if client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()

		dial := transport.DialContext
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if pconn, ok := ctx.Value(connKey).(*net.Conn); ok {
				*pconn = conn
			}
			return conn, err
		}
		transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			secure := ctx.Value(secureKey).(*tls.Config)
			tlsConn := tls.Client(conn, secure)
			if err = tlsConn.Handshake(); err != nil {
				return nil, err
			}
			if pconn, ok := ctx.Value(connKey).(*net.Conn); ok {
				*pconn = tlsConn
			}
			return tlsConn, err
		}

		client = &http.Client{
			Timeout:   time.Second * 5000,
			Transport: transport,
		}
	}

	user := req.URL.User

	// Hikvision won't answer on Basic auth with any headers
	if strings.HasPrefix(req.URL.Path, "/ISAPI/") {
		req.URL.User = nil
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusUnauthorized && user != nil {
		Close(res)

		auth := res.Header.Get("WWW-Authenticate")
		if !strings.HasPrefix(auth, "Digest") {
			return nil, errors.New("unsupported auth: " + auth)
		}

		realm := Between(auth, `realm="`, `"`)
		nonce := Between(auth, `nonce="`, `"`)
		qop := Between(auth, `qop="`, `"`)

		username := user.Username()
		password, _ := user.Password()
		ha1 := HexMD5(username, realm, password)

		uri := req.URL.RequestURI()
		ha2 := HexMD5(req.Method, uri)

		var header string

		switch qop {
		case "":
			response := HexMD5(ha1, nonce, ha2)
			header = fmt.Sprintf(
				`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
				username, realm, nonce, uri, response,
			)
		case "auth":
			nc := "00000001"
			cnonce := "00000001" // TODO: random...
			response := HexMD5(ha1, nonce, nc, cnonce, qop, ha2)
			header = fmt.Sprintf(
				`Digest username="%s", realm="%s", nonce="%s", uri="%s", qop=%s, nc=%s, cnonce="%s", response="%s"`,
				username, realm, nonce, uri, qop, nc, cnonce, response,
			)
		default:
			return nil, errors.New("unsupported qop: " + auth)
		}

		req.Header.Set("Authorization", header)

		if res, err = client.Do(req); err != nil {
			return nil, err
		}
	}

	return res, nil
}

var client *http.Client

type key string

var connKey = key("conn")
var secureKey = key("secure")

func WithConn() (context.Context, *net.Conn) {
	pconn := new(net.Conn)
	return context.WithValue(context.Background(), connKey, pconn), pconn
}

func Close(res *http.Response) {
	if res.Body != nil {
		_ = res.Body.Close()
	}
}

func IsIP(hostname string) bool {
	return net.ParseIP(hostname) != nil
}
