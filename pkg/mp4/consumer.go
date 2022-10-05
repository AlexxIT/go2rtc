package mp4

import (
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

type Consumer struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	muxer  *Muxer
	codecs []*streamer.Codec
	start  bool

	send int
}

func (c *Consumer) GetMedias() []*streamer.Media {
	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecH264, ClockRate: 90000},
				{Name: streamer.CodecH265, ClockRate: 90000},
			},
		},
		//{
		//	Kind:      streamer.KindAudio,
		//	Direction: streamer.DirectionRecvonly,
		//	Codecs: []*streamer.Codec{
		//		{Name: streamer.CodecAAC, ClockRate: 16000},
		//	},
		//},
	}
}

func (c *Consumer) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	codec := track.Codec
	switch codec.Name {
	case streamer.CodecH264:
		c.codecs = append(c.codecs, track.Codec)

		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if c.muxer == nil {
				return nil
			}

			if !c.start {
				if h264.IsKeyframe(packet.Payload) {
					c.start = true
				} else {
					return nil
				}
			}

			buf := c.muxer.Marshal(packet)
			c.send += len(buf)
			c.Fire(buf)

			return nil
		}

		var wrapper streamer.WrapperFunc
		if h264.IsAVC(codec) {
			wrapper = h264.RepairAVC(track)
		} else {
			wrapper = h264.RTPDepay(track)
		}
		push = wrapper(push)

		return track.Bind(push)

	case streamer.CodecH265:
		c.codecs = append(c.codecs, track.Codec)

		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if !c.start {
				if h265.IsKeyframe(packet.Payload) {
					c.start = true
				} else {
					return nil
				}
			}

			buf := c.muxer.Marshal(packet)
			c.send += len(buf)
			c.Fire(buf)

			return nil
		}

		if !h264.IsAVC(codec) {
			wrapper := h265.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)
	}

	fmt.Printf("[rtmp] unsupported codec: %+v\n", track.Codec)

	return nil
}

func (c *Consumer) MimeType() string {
	return c.muxer.MimeType(c.codecs)
}

func (c *Consumer) Init() ([]byte, error) {
	if c.muxer == nil {
		c.muxer = &Muxer{}
	}
	return c.muxer.GetInit(c.codecs)
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
