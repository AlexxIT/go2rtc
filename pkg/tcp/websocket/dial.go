package websocket

import (
	cryptorand "crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"net"
	"net/http"
	"strings"
)

func Dial(address string) (net.Conn, error) {
	if strings.HasPrefix(address, "ws") {
		address = "http" + address[2:] // support http and https
	}

	// using custom client for support Digest Auth
	// https://github.com/AlexxIT/go2rtc/issues/415
	ctx, pconn := tcp.WithConn()

	req, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, err
	}

	key, accept := GetKeyAccept()

	// Version, Key, Protocol important for Axis cameras
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Protocol", "binary")

	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusSwitchingProtocols {
		return nil, errors.New("wrong status: " + res.Status)
	}

	if res.Header.Get("Sec-Websocket-Accept") != accept {
		return nil, errors.New("wrong websocket accept")
	}

	return NewClient(*pconn), nil
}

func GetKeyAccept() (key, accept string) {
	b := make([]byte, 16)
	_, _ = cryptorand.Read(b)
	key = base64.StdEncoding.EncodeToString(b)

	h := sha1.New()
	h.Write([]byte(key))
	h.Write([]byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	accept = base64.StdEncoding.EncodeToString(h.Sum(nil))

	return
}
