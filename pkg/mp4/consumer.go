package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

type Consumer struct {
	streamer.Element

	Medias     []*streamer.Media
	UserAgent  string
	RemoteAddr string

	muxer  *Muxer
	codecs []*streamer.Codec
	start  bool

	send int
}

func (c *Consumer) GetMedias() []*streamer.Media {
	if c.Medias != nil {
		return c.Medias
	}

	// default medias
	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecH264},
				{Name: streamer.CodecH265},
			},
		},
		{
			Kind:      streamer.KindAudio,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecAAC},
			},
		},
	}
}

func (c *Consumer) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	trackID := byte(len(c.codecs))
	c.codecs = append(c.codecs, track.Codec)

	codec := track.Codec
	switch codec.Name {
	case streamer.CodecH264:
		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if !c.start {
				return nil
			}

			buf := c.muxer.Marshal(trackID, packet)
			c.send += len(buf)
			c.Fire(buf)

			return nil
		}

		var wrapper streamer.WrapperFunc
		if codec.IsRTP() {
			wrapper = h264.RTPDepay(track)
		} else {
			wrapper = h264.RepairAVC(track)
		}
		push = wrapper(push)

		return track.Bind(push)

	case streamer.CodecH265:
		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if !c.start {
				return nil
			}

			buf := c.muxer.Marshal(trackID, packet)
			c.send += len(buf)
			c.Fire(buf)

			return nil
		}

		if codec.IsRTP() {
			wrapper := h265.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)

	case streamer.CodecAAC:
		push := func(packet *rtp.Packet) error {
			if !c.start {
				return nil
			}

			buf := c.muxer.Marshal(trackID, packet)
			c.send += len(buf)
			c.Fire(buf)

			return nil
		}

		if codec.IsRTP() {
			wrapper := aac.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)
	}

	panic("unsupported codec")
}

func (c *Consumer) MimeType() string {
	return c.muxer.MimeType(c.codecs)
}

func (c *Consumer) Init() ([]byte, error) {
	c.muxer = &Muxer{}
	return c.muxer.GetInit(c.codecs)
}

func (c *Consumer) Start() {
	c.start = true
}

//

func (c *Consumer) MarshalJSON() ([]byte, error) {
	v := map[string]interface{}{
		"type":        "MP4 server consumer",
		"send":        c.send,
		"remote_addr": c.RemoteAddr,
		"user_agent":  c.UserAgent,
	}

	return json.Marshal(v)
}
