package rtmp

import (
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
)

func Dial(rawURL string) (core.Producer, error) {
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

	rd, err := NewReader(u, conn)
	if err != nil {
		return nil, err
	}

	return flv.Open(rd)
}
