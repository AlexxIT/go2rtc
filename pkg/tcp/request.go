package tcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Do - http.Client with support Digest Authorization
func Do(req *http.Request) (*http.Response, error) {
	var conn net.Conn

	client := http.Client{Timeout: time.Second * 5000}

	// for multipart requests return conn as Body (for write support)
	if ct := req.Header.Get("Content-Type"); strings.HasPrefix(ct, "multipart/mixed") {
		var d net.Dialer
		client.Transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				var err error
				conn, err = d.DialContext(ctx, network, addr)
				return conn, err
			},
		}
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode == http.StatusUnauthorized && req.URL.User != nil {
		auth := res.Header.Get("WWW-Authenticate")
		if !strings.HasPrefix(auth, "Digest") {
			return nil, errors.New("unsupported auth: " + auth)
		}

		realm := Between(auth, `realm="`, `"`)
		nonce := Between(auth, `nonce="`, `"`)
		qop := Between(auth, `qop="`, `"`)

		user := req.URL.User
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
				user, realm, nonce, uri, response,
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

		res, err = client.Do(req)
		if err != nil {
			return nil, err
		}
	}

	if conn != nil {
		res.Body = conn
	}

	return res, nil
}
