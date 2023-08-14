package rtmp

import (
	"bufio"
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
)

func Dial(rawURL string) (*flv.Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	host := u.Host
	if strings.IndexByte(host, ':') < 0 {
		host += ":1935"
	}

	conn, err := net.DialTimeout("tcp", host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	tr := &rtmp{
		url:  rawURL,
		conn: conn,
		rd:   bufio.NewReaderSize(conn, core.BufferSize),
	}

	if args := strings.Split(u.Path, "/"); len(args) >= 2 {
		tr.app = args[1]
		if len(args) >= 3 {
			tr.stream = args[2]
			if u.RawQuery != "" {
				tr.stream += "?" + u.RawQuery
			}
		}
	}

	if err = tr.init(); err != nil {
		return nil, err
	}

	return &flv.Client{Transport: tr, URL: rawURL}, nil
}
