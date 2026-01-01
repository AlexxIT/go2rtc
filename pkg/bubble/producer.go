package bubble

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	if c.medias == nil {
		c.medias = []*core.Media{
			{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{Name: c.videoCodec, ClockRate: 90000, PayloadType: core.PayloadTypeRAW},
				},
			},
			{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{Name: core.CodecPCMA, ClockRate: 8000, PayloadType: 8},
				},
			},
		}
	}

	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	track := core.NewReceiver(media, codec)

	switch media.Kind {
	case core.KindVideo:
		c.videoTrack = track
	case core.KindAudio:
		c.audioTrack = track
	}

	c.receivers = append(c.receivers, track)

	return track, nil
}

func (c *Client) Start() error {
	if err := c.Play(); err != nil {
		return err
	}
	return c.Handle()
}

func (c *Client) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.ID(c),
		FormatName: "bubble",
		Protocol:   "http",
		Medias:     c.medias,
		Recv:       c.recv,
		Receivers:  c.receivers,
	}
	if c.conn != nil {
		info.RemoteAddr = c.conn.RemoteAddr().String()
	}
	return json.Marshal(info)
}
