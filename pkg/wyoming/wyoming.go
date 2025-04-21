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

	cc := core.Connection{
		ID:         core.NewID(),
		FormatName: "wyoming",
		Medias: []*core.Media{
			{
				Kind: core.KindAudio,
				Codecs: []*core.Codec{
					{Name: core.CodecPCML, ClockRate: 16000},
				},
			},
		},
		Transport: conn,
	}

	if u.Query().Get("backchannel") != "1" {
		cc.Medias[0].Direction = core.DirectionRecvonly
		return &Producer{cc, NewAPI(conn)}, nil
	} else {
		cc.Medias[0].Direction = core.DirectionSendonly
		return &Backchannel{cc, NewAPI(conn)}, nil
	}
}
