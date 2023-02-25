package webrtc

import "github.com/AlexxIT/go2rtc/pkg/streamer"

func (c *Conn) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}
	return nil
}

func (c *Conn) Start() error {
	return nil
}

func (c *Conn) Stop() error {
	return c.pc.Close()
}
