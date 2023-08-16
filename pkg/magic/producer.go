package magic

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	return c.prod.GetMedias()
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	return c.prod.GetTrack(media, codec)
}

func (c *Client) Start() error {
	return c.prod.Start()
}

func (c *Client) Stop() (err error) {
	return c.prod.Stop()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.prod)
}
