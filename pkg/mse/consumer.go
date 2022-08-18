package mse

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/mp4f"
	"github.com/pion/rtp"
	"time"
)

const MsgTypeMSE = "mse"

type Consumer struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	muxer   *mp4f.Muxer
	streams []av.CodecData
	start   bool

	send int
}

func (c *Consumer) GetMedias() []*streamer.Media {
	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecH264, ClockRate: 90000},
			},
		}, {
			Kind:      streamer.KindAudio,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecAAC, ClockRate: 16000},
			},
		},
	}
}

func (c *Consumer) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	codec := track.Codec
	switch codec.Name {
	case streamer.CodecH264:
		idx := int8(len(c.streams))

		sps, pps := h264.GetParameterSet(codec.FmtpLine)
		stream, err := h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
		if err != nil {
			return nil
		}
		c.streams = append(c.streams, stream)

		pkt := av.Packet{Idx: idx, CompositionTime: time.Millisecond}

		ts2time := time.Second / time.Duration(codec.ClockRate)

		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			switch h264.NALUType(packet.Payload) {
			case h264.NALUTypeIFrame:
				c.start = true
				pkt.IsKeyFrame = true
			case h264.NALUTypePFrame:
				if !c.start {
					return nil
				}
			default:
				return nil
			}

			pkt.Data = packet.Payload
			newTime := time.Duration(packet.Timestamp) * ts2time
			if pkt.Time > 0 {
				pkt.Duration = newTime - pkt.Time
			}
			pkt.Time = newTime

			for _, buf := range c.muxer.WritePacketV5(pkt) {
				c.send += len(buf)
				c.Fire(buf)
			}

			return nil
		}

		if !h264.IsAVC(codec) {
			wrapper := h264.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)
	}

	panic("unsupported codec")
}

func (c *Consumer) Init() {
	c.muxer = mp4f.NewMuxer(nil)
	if err := c.muxer.WriteHeader(c.streams); err != nil {
		return
	}

	codecs, buf := c.muxer.GetInit(c.streams)
	c.Fire(&streamer.Message{Type: MsgTypeMSE, Value: codecs})

	c.send += len(buf)
	c.Fire(buf)
}

//

func (c *Consumer) MarshalJSON() ([]byte, error) {
	v := map[string]interface{}{
		"type":        "MSE server consumer",
		"send":        c.send,
		"remote_addr": c.RemoteAddr,
		"user_agent":  c.UserAgent,
	}

	return json.Marshal(v)
}
