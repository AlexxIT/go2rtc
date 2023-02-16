package rtsp

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
)

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

	track, err := c.SetupMedia(media, codec, true)
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
