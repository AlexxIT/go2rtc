package mp4

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
	"sync"
)

type Consumer struct {
	core.Listener

	Medias     []*core.Media
	UserAgent  string
	RemoteAddr string

	senders []*core.Sender

	muxer *Muxer
	mu    sync.Mutex
	wait  byte

	send int
}

func (c *Consumer) GetMedias() []*core.Media {
	if c.Medias == nil {
		// default local medias
		c.Medias = []*core.Media{
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

	return c.Medias
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	trackID := byte(len(c.senders))

	codec := track.Codec.Clone()
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		c.wait = waitInit

		handler.Handler = func(packet *rtp.Packet) {
			if packet.Version != h264.RTPPacketVersionAVC {
				return
			}

			if c.wait != waitNone {
				if c.wait == waitInit || !h264.IsKeyframe(packet.Payload) {
					return
				}
				c.wait = waitNone
			}

			// important to use Mutex because right fragment order
			c.mu.Lock()
			buf := c.muxer.Marshal(trackID, packet)
			c.Fire(buf)
			c.send += len(buf)
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVC(track.Codec, handler.Handler)
		}

	case core.CodecH265:
		c.wait = waitInit

		handler.Handler = func(packet *rtp.Packet) {
			if packet.Version != h264.RTPPacketVersionAVC {
				return
			}

			if c.wait != waitNone {
				if c.wait == waitInit || !h265.IsKeyframe(packet.Payload) {
					return
				}
				c.wait = waitNone
			}

			c.mu.Lock()
			buf := c.muxer.Marshal(trackID, packet)
			c.Fire(buf)
			c.send += len(buf)
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h265.RTPDepay(track.Codec, handler.Handler)
		}

	default:
		handler.Handler = func(packet *rtp.Packet) {
			if c.wait != waitNone {
				return
			}

			c.mu.Lock()
			buf := c.muxer.Marshal(trackID, packet)
			c.Fire(buf)
			c.send += len(buf)
			c.mu.Unlock()
		}

		switch track.Codec.Name {
		case core.CodecAAC:
			if track.Codec.IsRTP() {
				handler.Handler = aac.RTPDepay(handler.Handler)
			}
		case core.CodecOpus, core.CodecMP3: // no changes
		case core.CodecPCMA, core.CodecPCMU, core.CodecPCM:
			handler.Handler = pcm.FLACEncoder(track.Codec, handler.Handler)
			codec.Name = core.CodecFLAC

		default:
			handler.Handler = nil
		}
	}

	if handler.Handler == nil {
		println("ERROR: MP4 unsupported codec: " + track.Codec.String())
		return nil
	}

	handler.HandleRTP(track)
	c.senders = append(c.senders, handler)

	return nil
}

func (c *Consumer) Stop() error {
	for _, sender := range c.senders {
		sender.Close()
	}
	return nil
}

func (c *Consumer) Codecs() []*core.Codec {
	codecs := make([]*core.Codec, len(c.senders))
	for i, sender := range c.senders {
		codecs[i] = sender.Codec
	}
	return codecs
}

func (c *Consumer) MimeCodecs() string {
	return c.muxer.MimeCodecs(c.Codecs())
}

func (c *Consumer) MimeType() string {
	return `video/mp4; codecs="` + c.MimeCodecs() + `"`
}

func (c *Consumer) Init() ([]byte, error) {
	c.muxer = &Muxer{}
	return c.muxer.GetInit(c.Codecs())
}

func (c *Consumer) Start() {
	if c.wait == waitInit {
		c.wait = waitKeyframe
	}
}

func (c *Consumer) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "MP4 passive consumer",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Medias:     c.Medias,
		Senders:    c.senders,
		Send:       c.send,
	}
	return json.Marshal(info)
}
