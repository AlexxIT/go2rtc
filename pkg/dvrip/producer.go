package dvrip

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}
	return nil, core.ErrCantGetTrack
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() error {
	for _, receiver := range c.receivers {
		receiver.Close()
	}
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "DVRIP active producer",
		RemoteAddr: c.conn.RemoteAddr().String(),
		Medias:     c.medias,
		Receivers:  c.receivers,
		Recv:       int(c.recv),
	}
	return json.Marshal(info)
}
