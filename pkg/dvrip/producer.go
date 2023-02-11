package dvrip

import "github.com/AlexxIT/go2rtc/pkg/streamer"

func (c *Client) GetMedias() []*streamer.Media {
	return c.medias
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	if c.videoTrack != nil && c.videoTrack.Codec == codec {
		return c.videoTrack
	}
	if c.audioTrack != nil && c.audioTrack.Codec == codec {
		return c.audioTrack
	}
	return nil
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() error {
	return c.Close()
}
