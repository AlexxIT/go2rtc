package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"sync/atomic"
)

type Consumer struct {
	streamer.Element

	Medias     []*streamer.Media
	UserAgent  string
	RemoteAddr string

	muxer  *Muxer
	codecs []*streamer.Codec
	wait   byte

	send uint32
}

// ParseQuery - like usual parse, but with mp4 param handler
func ParseQuery(query map[string][]string) []*streamer.Media {
	if query["mp4"] != nil {
		cons := Consumer{}
		return cons.GetMedias()
	}

	return streamer.ParseQuery(query)
}

const (
	waitNone byte = iota
	waitKeyframe
	waitInit
)

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
		c.wait = waitInit

		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if c.wait != waitNone {
				if c.wait == waitInit || !h264.IsKeyframe(packet.Payload) {
					return nil
				}
				c.wait = waitNone
			}

			buf := c.muxer.Marshal(trackID, packet)
			atomic.AddUint32(&c.send, uint32(len(buf)))
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
		c.wait = waitInit

		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if c.wait != waitNone {
				if c.wait == waitInit || !h265.IsKeyframe(packet.Payload) {
					return nil
				}
				c.wait = waitNone
			}

			buf := c.muxer.Marshal(trackID, packet)
			atomic.AddUint32(&c.send, uint32(len(buf)))
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
			if c.wait != waitNone {
				return nil
			}

			buf := c.muxer.Marshal(trackID, packet)
			atomic.AddUint32(&c.send, uint32(len(buf)))
			c.Fire(buf)

			return nil
		}

		if codec.IsRTP() {
			wrapper := aac.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)

	case streamer.CodecOpus, streamer.CodecMP3, streamer.CodecPCMU, streamer.CodecPCMA:
		push := func(packet *rtp.Packet) error {
			if c.wait != waitNone {
				return nil
			}

			buf := c.muxer.Marshal(trackID, packet)
			atomic.AddUint32(&c.send, uint32(len(buf)))
			c.Fire(buf)

			return nil
		}

		return track.Bind(push)
	}

	panic("unsupported codec")
}

func (c *Consumer) MimeCodecs() string {
	return c.muxer.MimeCodecs(c.codecs)
}

func (c *Consumer) MimeType() string {
	return `video/mp4; codecs="` + c.MimeCodecs() + `"`
}

func (c *Consumer) Init() ([]byte, error) {
	c.muxer = &Muxer{}
	return c.muxer.GetInit(c.codecs)
}

func (c *Consumer) Start() {
	if c.wait == waitInit {
		c.wait = waitKeyframe
	}
}

//

func (c *Consumer) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:       "MP4 client",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Send:       atomic.LoadUint32(&c.send),
	}
	return json.Marshal(info)
}
