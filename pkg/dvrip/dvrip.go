package dvrip

import "github.com/AlexxIT/go2rtc/pkg/core"

func Dial(url string) (core.Producer, error) {
	client := &Client{}
	if err := client.Dial(url); err != nil {
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
