package http

import (
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/rtmp"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"net/http"
	"strings"
)

func Init() {
	streams.HandleFunc("http", handle)
	streams.HandleFunc("https", handle)
	streams.HandleFunc("httpx", handle)
}

func handle(url string) (core.Producer, error) {
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

	case "video/mpeg":
		client := mpegts.NewClient(res)
		if err = client.Handle(); err != nil {
			return nil, err
		}
		return client, nil
	}

	return nil, fmt.Errorf("unsupported Content-Type: %s", ct)
}
