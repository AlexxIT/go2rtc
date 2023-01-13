package ivideon

import (
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func (c *Client) GetMedias() []*streamer.Media {
	return c.medias
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}
	panic(fmt.Sprintf("wrong media/codec: %+v %+v", media, codec))
}

func (c *Client) Start() error {
	err := c.Handle()
	if c.buffer == nil {
		return nil
	}
	return err
}

func (c *Client) Stop() error {
	return c.Close()
}
