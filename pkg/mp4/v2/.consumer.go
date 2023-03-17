package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"sync/atomic"
)

type Consumer struct {
	core.Listener

	Medias     []*core.Media
	UserAgent  string
	RemoteAddr string

	muxer  *Muxer
	codecs []*core.Codec
	wait   byte

	send uint32
}

const (
	waitNone byte = iota
	waitKeyframe
	waitInit
)

func (c *Consumer) GetMedias() []*core.Media {
	if c.Medias != nil {
		return c.Medias
	}

	// default medias
	return []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
				{Name: core.CodecH265},
			},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecAAC},
			},
		},
	}
}

func (c *Consumer) AddTrack(media *core.Media, track *core.Track) *core.Track {
	trackID := byte(len(c.codecs))
	c.codecs = append(c.codecs, track.Codec)

	codec := track.Codec
	switch codec.Name {
	case core.CodecH264:
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

	case core.CodecH265:
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
	if c.wait == waitInit {
		c.wait = waitKeyframe
	}
}

//

func (c *Consumer) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "MP4 client",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Send:       atomic.LoadUint32(&c.send),
	}
	return json.Marshal(info)
}
