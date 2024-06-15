package ivideon

import (
	"encoding/json"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func (c *Client) GetMedias() []*core.Media {
	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if c.receiver != nil {
		return c.receiver, nil
	}
	return nil, core.ErrCantGetTrack
}

func (c *Client) Start() error {
	err := c.Handle()
	if c.buffer == nil {
		return nil
	}
	return err
}

func (c *Client) Stop() error {
	if c.receiver != nil {
		c.receiver.Close()
	}
	return c.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Connection{
		ID:         core.ID(c),
		FormatName: "ivideon",
		Protocol:   "ws",
		URL:        c.ID,
		Medias:     c.medias,
		Recv:       c.recv,
	}
	if c.conn != nil {
		info.RemoteAddr = c.conn.RemoteAddr().String()
	}
	if c.receiver != nil {
		info.Receivers = []*core.Receiver{c.receiver}
	}
	return json.Marshal(info)
}
