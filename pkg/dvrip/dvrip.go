package dvrip

import (
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func Dial(rawURL string) (core.Producer, error) {
	client := &Client{}
	if err := client.Dial(rawURL); err != nil {
		return nil, err
	}

	conn := core.Connection{
		ID:         core.NewID(),
		FormatName: "dvrip",
		Protocol:   "tcp",
		RemoteAddr: client.conn.RemoteAddr().String(),
		Transport:  client.conn,
	}

	if client.stream != "" {
		prod := &Producer{Connection: conn, client: client}

		if u, err := url.Parse(rawURL); err == nil {
			prod.Media = u.Query().Get("media")
		}

		if err := prod.probe(); err != nil {
			return nil, err
		}
		return prod, nil
	} else {
		conn.Medias = []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					// leave only one codec here for better compatibility with cameras
					// https://github.com/AlexxIT/go2rtc/issues/1111
					{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
				},
			},
		}
		return &Backchannel{Connection: conn, client: client}, nil
	}
}
