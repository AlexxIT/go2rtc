package roborock

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	return c.conn.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if media.Kind == core.KindAudio {
		c.audio = true
	}

	return c.conn.GetTrack(media, codec)
}

func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	c.backchannel = true
	return c.conn.AddTrack(media, codec, track)
}

func (c *Client) Start() error {
	if c.audio || c.backchannel {
		if err := c.StartVoiceChat(); err != nil {
			return err
		}

		if c.backchannel {
			if err := c.SetVoiceChatVolume(80); err != nil {
				return err
			}
		}
	}
	return c.conn.Start()
}

func (c *Client) Stop() error {
	_ = c.iot.Close()
	return c.conn.Stop()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return c.conn.MarshalJSON()
}
