package http

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/magic"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/multipart"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
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
	case "image/jpeg":
		return mjpeg.NewClient(res), nil

	case "multipart/x-mixed-replace":
		return multipart.NewClient(res)

	case "video/x-flv":
		var client *flv.Client
		if client, err = flv.NewClient(res.Body); err != nil {
			return nil, err
		}
		if err = client.Describe(); err != nil {
			return nil, err
		}
		client.URL = url
		return client, nil

	default: // "video/mpeg":
	}

	client, err := magic.Open(res.Body)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func handleTCP(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	return magic.Open(conn)
}
