package dvrip

import "github.com/AlexxIT/go2rtc/pkg/core"

func Dial(url string) (core.Producer, error) {
	client := &Client{}
	if err := client.Dial(url); err != nil {
		return nil, err
	}

	if client.stream != "" {
		prod := &Producer{client: client}
		prod.Type = "DVRIP active producer"
		if err := prod.probe(); err != nil {
			return nil, err
		}
		return prod, nil
	} else {
		cons := &Consumer{client: client}
		cons.Type = "DVRIP active consumer"
		cons.Medias = []*core.Media{
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs: []*core.Codec{
					{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
					{Name: core.CodecPCMU, ClockRate: 8000, PayloadType: 0},
				},
			},
		}
		return cons, nil
	}
}
