package rtsp

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

// Element Producer

func (c *Conn) GetMedias() []*streamer.Media {
	if c.Medias != nil {
		return c.Medias
	}

	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecAll},
			},
		},
		{
			Kind:      streamer.KindAudio,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecAll},
			},
		},
	}
}

func (c *Conn) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}

	// can't setup new tracks from play state - forcing a reconnection feature
	switch c.state {
	case StatePlay, StateHandle:
		go c.Close()
		return streamer.NewTrack(codec, media.Direction)
	}

	track, err := c.SetupMedia(media, codec)
	if err != nil {
		return nil
	}
	return track
}

func (c *Conn) Start() error {
	switch c.mode {
	case ModeClientProducer:
		if err := c.Play(); err != nil {
			return err
		}
	case ModeServerProducer:
	default:
		return fmt.Errorf("start wrong mode: %d", c.mode)
	}

	return c.Handle()
}

func (c *Conn) Stop() error {
	return c.Close()
}

// Consumer

func (c *Conn) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	switch track.Direction {
	// send our track to RTSP consumer (ex. FFmpeg)
	case streamer.DirectionSendonly:
		i := len(c.tracks)
		channelID := byte(i << 1)

		codec := track.Codec.Clone()
		codec.PayloadType = uint8(96 + i)

		if media.MatchAll() {
			// fill consumer medias list
			c.Medias = append(c.Medias, &streamer.Media{
				Kind: media.Kind, Direction: media.Direction,
				Codecs: []*streamer.Codec{codec},
			})
		} else {
			// find consumer media and replace codec with right one
			for i, m := range c.Medias {
				if m == media {
					media.Codecs = []*streamer.Codec{codec}
					c.Medias[i] = media
					break
				}
			}
		}

		track = c.bindTrack(track, channelID, codec.PayloadType)
		track.Codec = codec
		c.tracks = append(c.tracks, track)

		return track

	case streamer.DirectionRecvonly:
		panic("not implemented")
	}

	panic("wrong direction")
}

//

func (c *Conn) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		UserAgent: c.UserAgent,
		Medias:    c.Medias,
		Tracks:    c.tracks,
		Recv:      uint32(c.receive),
		Send:      uint32(c.send),
	}

	switch c.mode {
	case ModeUnknown:
		info.Type = "RTSP unknown"
	case ModeClientProducer, ModeServerProducer:
		info.Type = "RTSP source"
	case ModeServerConsumer:
		info.Type = "RTSP client"
	}

	if c.URL != nil {
		info.URL = c.URL.String()
	}
	if c.conn != nil {
		info.RemoteAddr = c.conn.RemoteAddr().String()
	}

	//for i, track := range c.tracks {
	//	k := "track:" + strconv.Itoa(i+1)
	//	if track.MimeType() == streamer.MimeTypeH264 {
	//		v[k] = h264.Describe(track.Caps())
	//	} else {
	//		v[k] = track.MimeType()
	//	}
	//}

	return json.Marshal(info)
}
