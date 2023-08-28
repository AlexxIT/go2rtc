package http

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hls"
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

func handleHTTP(rawURL string) (core.Producer, error) {
	rawURL, rawQuery, _ := strings.Cut(rawURL, "#")

	// first we get the Content-Type to define supported producer
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}

	var chunked bool

	if rawQuery != "" {
		query := streams.ParseQuery(rawQuery)

		for _, header := range query["header"] {
			key, value, _ := strings.Cut(header, ":")
			req.Header.Add(key, strings.TrimSpace(value))
		}

		chunked = query.Get("chunked") == "1"
	}

	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New(res.Status)
	}

	// 1. Guess format from content type
	ct := res.Header.Get("Content-Type")
	if i := strings.IndexByte(ct, ';'); i > 0 {
		ct = ct[:i]
	}

	var ext string
	if i := strings.LastIndexByte(req.URL.Path, '.'); i > 0 {
		ext = req.URL.Path[i+1:]
	}

	var rd io.ReadCloser

	// support buggy clients, like TP-Link cameras with HTTP/1.0 chunked encoding
	if chunked {
		rd = struct {
			io.Reader
			io.Closer
		}{
			httputil.NewChunkedReader(res.Body),
			res.Body,
		}
	} else {
		rd = res.Body
	}

	switch {
	case ct == "image/jpeg":
		return mjpeg.NewClient(res), nil

	case ct == "multipart/x-mixed-replace":
		return multipart.Open(rd)

	case ct == "application/vnd.apple.mpegurl" || ext == "m3u8":
		return hls.OpenURL(req.URL, rd)
	}

	return magic.Open(rd)
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
