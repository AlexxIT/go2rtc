package magic

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if c.receiver == nil {
		c.receiver = core.NewReceiver(media, codec)
	}
	return c.receiver, nil
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() (err error) {
	if c.receiver != nil {
		c.receiver.Close()
	}
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:   c.Desc,
		URL:    c.URL,
		Medias: c.medias,
		Recv:   c.recv,
	}
	if c.receiver != nil {
		info.Receivers = append(info.Receivers, c.receiver)
	}
	return json.Marshal(info)
}
