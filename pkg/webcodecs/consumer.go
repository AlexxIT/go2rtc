package webcodecs

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

// Binary frame header (9 bytes):
// Byte 0:    flags (bit7=video, bit6=keyframe, bits0-5=trackID)
// Byte 1-8:  timestamp in microseconds (uint64 BE)
// Byte 9+:   payload

const headerSize = 9

type Consumer struct {
	core.Connection
	wr    *core.WriteBuffer
	mu    sync.Mutex
	start atomic.Bool

	UseGOP bool
}

type InitInfo struct {
	Video *VideoInfo `json:"video,omitempty"`
	Audio *AudioInfo `json:"audio,omitempty"`
}

type VideoInfo struct {
	Codec string `json:"codec"`
}

type AudioInfo struct {
	Codec      string `json:"codec"`
	SampleRate int    `json:"sampleRate"`
	Channels   int    `json:"channels"`
}

func NewConsumer(medias []*core.Media) *Consumer {
	if medias == nil {
		medias = []*core.Media{
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
					{Name: core.CodecOpus},
					{Name: core.CodecPCMA},
					{Name: core.CodecPCMU},
				},
			},
		}
	}

	wr := core.NewWriteBuffer(nil)
	return &Consumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "webcodecs",
			Medias:     medias,
			Transport:  wr,
		},
		wr: wr,
	}
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	trackID := byte(len(c.Senders))

	codec := track.Codec.Clone()
	handler := core.NewSender(media, codec)

	switch track.Codec.Name {
	case core.CodecH264:
		clockRate := codec.ClockRate
		handler.Handler = func(packet *rtp.Packet) {
			keyframe := h264.IsKeyframe(packet.Payload)
			if !c.start.Load() {
				if !keyframe {
					return
				}
				c.start.Store(true)
			}

			payload := annexb.DecodeAVCC(packet.Payload, true)
			flags := byte(0x80) | trackID // video flag
			if keyframe {
				flags |= 0x40 // keyframe flag
			}

			c.mu.Lock()
			msg := buildFrame(flags, rtpToMicroseconds(packet.Timestamp, clockRate), payload)
			if n, err := c.wr.Write(msg); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVCC(track.Codec, handler.Handler)
		}

	case core.CodecH265:
		clockRate := codec.ClockRate
		handler.Handler = func(packet *rtp.Packet) {
			keyframe := h265.IsKeyframe(packet.Payload)
			if !c.start.Load() {
				if !keyframe {
					return
				}
				c.start.Store(true)
			}

			payload := annexb.DecodeAVCC(packet.Payload, true)
			flags := byte(0x80) | trackID // video flag
			if keyframe {
				flags |= 0x40 // keyframe flag
			}

			c.mu.Lock()
			msg := buildFrame(flags, rtpToMicroseconds(packet.Timestamp, clockRate), payload)
			if n, err := c.wr.Write(msg); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h265.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h265.RepairAVCC(track.Codec, handler.Handler)
		}

	default:
		clockRate := codec.ClockRate
		handler.Handler = func(packet *rtp.Packet) {
			if !c.start.Load() {
				return
			}

			flags := trackID // audio flag (bit7=0)

			c.mu.Lock()
			msg := buildFrame(flags, rtpToMicroseconds(packet.Timestamp, clockRate), packet.Payload)
			if n, err := c.wr.Write(msg); err == nil {
				c.Send += n
			}
			c.mu.Unlock()
		}

		switch track.Codec.Name {
		case core.CodecAAC:
			if track.Codec.IsRTP() {
				handler.Handler = aac.RTPDepay(handler.Handler)
			}
		case core.CodecOpus, core.CodecPCMA, core.CodecPCMU:
			// pass through directly — WebCodecs decodes these natively
		default:
			handler.Handler = nil
		}
	}

	if handler.Handler == nil {
		s := "webcodecs: unsupported codec: " + track.Codec.String()
		println(s)
		return errors.New(s)
	}

	handler.HandleRTP(track)
	c.Senders = append(c.Senders, handler)

	return nil
}

func (c *Consumer) GetInitInfo() *InitInfo {
	info := &InitInfo{}

	for _, sender := range c.Senders {
		codec := sender.Codec
		switch codec.Name {
		case core.CodecH264:
			info.Video = &VideoInfo{
				Codec: "avc1." + h264.GetProfileLevelID(codec.FmtpLine),
			}
		case core.CodecH265:
			info.Video = &VideoInfo{
				Codec: "hvc1.1.6.L153.B0",
			}
		case core.CodecAAC:
			channels := int(codec.Channels)
			if channels == 0 {
				channels = 1
			}
			info.Audio = &AudioInfo{
				Codec:      "mp4a.40.2",
				SampleRate: int(codec.ClockRate),
				Channels:   channels,
			}
		case core.CodecOpus:
			channels := int(codec.Channels)
			if channels == 0 {
				channels = 2
			}
			info.Audio = &AudioInfo{
				Codec:      "opus",
				SampleRate: int(codec.ClockRate),
				Channels:   channels,
			}
		case core.CodecPCMA:
			info.Audio = &AudioInfo{
				Codec:      "alaw",
				SampleRate: int(codec.ClockRate),
				Channels:   1,
			}
		case core.CodecPCMU:
			info.Audio = &AudioInfo{
				Codec:      "ulaw",
				SampleRate: int(codec.ClockRate),
				Channels:   1,
			}
		}
	}

	return info
}

func (c *Consumer) WriteTo(wr io.Writer) (int64, error) {
	if len(c.Senders) == 1 && c.Senders[0].Codec.IsAudio() {
		c.start.Store(true)
	}

	return c.wr.WriteTo(wr)
}

func buildFrame(flags byte, timestamp uint64, payload []byte) []byte {
	msg := make([]byte, headerSize+len(payload))
	msg[0] = flags
	binary.BigEndian.PutUint64(msg[1:9], timestamp)
	copy(msg[headerSize:], payload)
	return msg
}

func rtpToMicroseconds(timestamp uint32, clockRate uint32) uint64 {
	if clockRate == 0 {
		return uint64(timestamp)
	}
	return uint64(timestamp) * 1_000_000 / uint64(clockRate)
}
