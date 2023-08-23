package rtmp

import (
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
)

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := tcp.Dial(u, "1935", core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	rd, err := NewReader(u, conn)
	if err != nil {
		return nil, err
	}

	return flv.Open(rd)
}
