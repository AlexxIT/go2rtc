package webrtc

import (
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func (c *Conn) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	if c.Mode != streamer.ModeActiveProducer && c.Mode != streamer.ModePassiveProducer {
		panic("not implemented")
	}

	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}

	track := streamer.NewTrack(media, codec)

	if media.Direction == streamer.DirectionRecvonly {
		track = c.addSendTrack(media, track)
	}

	c.tracks = append(c.tracks, track)
	return track
}

func (c *Conn) Start() error {
	c.closed.Wait()
	return nil
}

func (c *Conn) Stop() error {
	return c.pc.Close()
}
