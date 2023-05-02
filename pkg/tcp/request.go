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
	if secureClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()

		dial := transport.DialContext
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dial(ctx, network, addr)
			if pconn, ok := ctx.Value(connKey).(*net.Conn); ok {
				*pconn = conn
			}
			return conn, err
		}

		secureClient = &http.Client{
			Timeout:   time.Second * 5000,
			Transport: transport,
		}
	}

	var client *http.Client

	if req.URL.Scheme == "httpx" {
		req.URL.Scheme = "https"

		if insecureClient == nil {
			transport := secureClient.Transport.(*http.Transport).Clone()
			transport.TLSClientConfig.InsecureSkipVerify = true

			insecureClient = &http.Client{
				Timeout:   secureClient.Timeout,
				Transport: transport,
			}
		}

		client = insecureClient
	} else {
		client = secureClient
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

		if res, err = client.Do(req); err != nil {
			return nil, err
		}
	}

	return res, nil
}

var secureClient, insecureClient *http.Client
var connKey struct{}

func WithConn() (context.Context, *net.Conn) {
	pconn := new(net.Conn)
	return context.WithValue(context.Background(), connKey, pconn), pconn
}

func Close(res *http.Response) {
	if res.Body != nil {
		_ = res.Body.Close()
	}
}
