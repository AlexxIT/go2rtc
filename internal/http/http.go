package http

import (
	"errors"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/rtmp"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Init() {
	streams.HandleFunc("http", handleHTTP)
	streams.HandleFunc("https", handleHTTP)
	streams.HandleFunc("httpx", handleHTTP)

	streams.HandleFunc("tcp", handleTCP)
}

func handleHTTP(url string) (core.Producer, error) {
	// first we get the Content-Type to define supported producer
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	ct := res.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i > 0 {
		ct = ct[:i]
	}

	switch ct {
	case "image/jpeg", "multipart/x-mixed-replace":
		return mjpeg.NewClient(res), nil

	case "video/x-flv":
		var conn *rtmp.Client
		if conn, err = rtmp.Accept(res); err != nil {
			return nil, err
		}
		if err = conn.Describe(); err != nil {
			return nil, err
		}
		return conn, nil

	default: // "video/mpeg":
	}

	client := magic.NewClient(res.Body)
	if err = client.Probe(); err != nil {
		return nil, err
	}

	client.Desc = "HTTP active producer"
	client.URL = url

	return client, nil
}

func handleTCP(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", u.Host, time.Second*3)
	if err != nil {
		return nil, err
	}

	client := magic.NewClient(conn)
	if err = client.Probe(); err != nil {
		return nil, err
	}

	client.Desc = "TCP active producer"
	client.URL = rawURL

	return client, nil
}
