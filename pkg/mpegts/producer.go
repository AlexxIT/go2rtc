package mpegts

import (
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
	return nil
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() error {
	return c.Close()
}
