package tcp

import (
	"github.com/AlexxIT/go2rtc/cmd/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"net"
	"net/http"
	"net/url"
	"time"
)

func Init() {
	streams.HandleFunc("tcp", handle)
}

func handle(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", u.Host, time.Second*3)
	if err != nil {
		return nil, err
	}

	req := &http.Request{URL: u}
	res := &http.Response{Body: conn, Request: req}
	client := mpegts.NewClient(res)
	if err := client.Handle(); err != nil {
		return nil, err
	}
	return client, nil
}
