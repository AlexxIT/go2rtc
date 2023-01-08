package rtsp

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"strconv"
)

// Element Producer

func (c *Conn) GetMedias() []*streamer.Media {
	return c.Medias
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

		for i, m := range c.Medias {
			if m == media {
				media.Codecs = []*streamer.Codec{codec}
				c.Medias[i] = media
				break
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
	v := map[string]interface{}{
		streamer.JSONReceive: c.receive,
		streamer.JSONSend:    c.send,
	}
	switch c.mode {
	case ModeUnknown:
		v[streamer.JSONType] = "RTSP unknown"
	case ModeClientProducer:
		v[streamer.JSONType] = "RTSP client producer"
	case ModeServerProducer:
		v[streamer.JSONType] = "RTSP server producer"
	case ModeServerConsumer:
		v[streamer.JSONType] = "RTSP server consumer"
	}
	//if c.URI != "" {
	//	v["uri"] = c.URI
	//}
	if c.URL != nil {
		v["url"] = c.URL.String()
	}
	if c.conn != nil {
		v[streamer.JSONRemoteAddr] = c.conn.RemoteAddr().String()
	}
	if c.UserAgent != "" {
		v[streamer.JSONUserAgent] = c.UserAgent
	}
	for i, media := range c.Medias {
		k := "media:" + strconv.Itoa(i)
		v[k] = media.String()
	}
	for i, track := range c.tracks {
		k := "track:" + strconv.Itoa(int(i>>1))
		v[k] = track.String()
	}
	//for i, track := range c.tracks {
	//	k := "track:" + strconv.Itoa(i+1)
	//	if track.MimeType() == streamer.MimeTypeH264 {
	//		v[k] = h264.Describe(track.Caps())
	//	} else {
	//		v[k] = track.MimeType()
	//	}
	//}
	return json.Marshal(v)
}
