package tapo

import (
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

func (c *Client) GetMedias() []*streamer.Media {
	// producer should have persistent medias
	if c.medias == nil {
		// don't know if all Tapo has this capabilities...
		c.medias = []*streamer.Media{
			{
				Kind:      streamer.KindVideo,
				Direction: streamer.DirectionSendonly,
				Codecs: []*streamer.Codec{
					{Name: streamer.CodecH264, ClockRate: 90000, PayloadType: streamer.PayloadTypeRAW},
				},
			},
			{
				Kind:      streamer.KindAudio,
				Direction: streamer.DirectionSendonly,
				Codecs: []*streamer.Codec{
					{Name: streamer.CodecPCMA, ClockRate: 8000, PayloadType: 8},
				},
			},
			{
				Kind:      streamer.KindAudio,
				Direction: streamer.DirectionRecvonly,
				Codecs: []*streamer.Codec{
					{Name: streamer.CodecPCMA, ClockRate: 8000, PayloadType: 8},
				},
			},
		}
	}

	return c.medias
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) (track *streamer.Track) {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}

	if c.tracks == nil {
		c.tracks = map[byte]*streamer.Track{}
	}

	if media.Kind == streamer.KindVideo {
		if err := c.SetupStream(); err != nil {
			return nil
		}

		track = streamer.NewTrack2(media, codec)
		c.tracks[mpegts.StreamTypeH264] = track
	} else {
		if media.Direction == streamer.DirectionSendonly {
			if err := c.SetupStream(); err != nil {
				return nil
			}

			track = streamer.NewTrack2(media, codec)
			c.tracks[mpegts.StreamTypePCMATapo] = track
		} else {
			if err := c.SetupBackchannel(); err != nil {
				return nil
			}

			if w := c.backchannelWriter(); w != nil {
				track = streamer.NewTrack2(media, codec)
				track.Bind(w)
				c.tracks[0] = track
			}
		}
	}

	return
}

func (c *Client) Start() error {
	return c.Handle()
}

func (c *Client) Stop() error {
	return c.Close()
}
