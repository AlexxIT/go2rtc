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

	rd := &rtmp{
		url:     rawURL,
		headers: map[uint32]*header{},
		conn:    conn,
		rd:      bufio.NewReaderSize(conn, core.BufferSize),
	}

	if args := strings.Split(u.Path, "/"); len(args) >= 2 {
		rd.app = args[1]
		if len(args) >= 3 {
			rd.stream = args[2]
			if u.RawQuery != "" {
				rd.stream += "?" + u.RawQuery
			}
		}
	}

	if err = rd.handshake(); err != nil {
		return nil, err
	}
	if err = rd.sendConfig(); err != nil {
		return nil, err
	}
	if err = rd.sendConnect(); err != nil {
		return nil, err
	}
	if err = rd.sendPlay(); err != nil {
		return nil, err
	}

	rd.buf = []byte{
		'F', 'L', 'V', // signature
		1,          // version
		0,          // flags (has video/audio)
		0, 0, 0, 9, // header size
	}

	return flv.Open(rd)
}
