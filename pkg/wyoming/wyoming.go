package wyoming

import (
	"net"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func Dial(rawURL string) (core.Producer, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialTimeout("tcp", u.Host, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}

	if u.Query().Get("backchannel") != "1" {
		return newProducer(conn), nil
	} else {
		return newBackchannel(conn), nil
	}
}
