package isapi

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

func (c *Client) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	consCodec := media.MatchCodec(track.Codec)
	consTrack := c.GetTrack(media, consCodec)
	if consTrack == nil {
		return nil
	}

	return track.Bind(func(packet *rtp.Packet) error {
		return consTrack.WriteRTP(packet)
	})
}
