package mp4

import (
	"encoding/hex"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/mp4f"
	"github.com/pion/rtp"
	"time"
)

type Consumer struct {
	streamer.Element

	Medias     []*streamer.Media
	UserAgent  string
	RemoteAddr string

	muxer    *mp4f.Muxer
	streams  []av.CodecData
	mimeType string
	start    bool

	send int
}

func (c *Consumer) GetMedias() []*streamer.Media {
	if c.Medias != nil {
		return c.Medias
	}

	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecH264, ClockRate: 90000},
			},
		},
		{
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
	trackID := int8(len(c.streams))

	switch codec.Name {
	case streamer.CodecH264:
		sps, pps := h264.GetParameterSet(codec.FmtpLine)
		stream, err := h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
		if err != nil {
			return nil
		}

		c.mimeType += "avc1." + h264.GetProfileLevelID(codec.FmtpLine)
		c.streams = append(c.streams, stream)

		pkt := av.Packet{Idx: trackID, CompositionTime: time.Millisecond}

		ts2time := time.Second / time.Duration(codec.ClockRate)

		push := func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return nil
			}

			if !c.start {
				return nil
			}

			pkt.Data = packet.Payload
			newTime := time.Duration(packet.Timestamp) * ts2time
			if pkt.Time > 0 {
				pkt.Duration = newTime - pkt.Time
			}
			pkt.Time = newTime

			ready, buf, _ := c.muxer.WritePacket(pkt, false)
			if ready {
				c.send += len(buf)
				c.Fire(buf)
			}

			return nil
		}

		if codec.IsRTP() {
			wrapper := h264.RTPDepay(track)
			push = wrapper(push)
		}

		return track.Bind(push)

	case streamer.CodecAAC:
		s := streamer.Between(codec.FmtpLine, "config=", ";")

		b, err := hex.DecodeString(s)
		if err != nil {
			return nil
		}

		stream, err := aacparser.NewCodecDataFromMPEG4AudioConfigBytes(b)
		if err != nil {
			return nil
		}

		c.mimeType += ",mp4a.40.2"
		c.streams = append(c.streams, stream)

		pkt := av.Packet{Idx: trackID, CompositionTime: time.Millisecond}

		ts2time := time.Second / time.Duration(codec.ClockRate)

		push := func(packet *rtp.Packet) error {
			if !c.start {
				return nil
			}

			pkt.Data = packet.Payload
			newTime := time.Duration(packet.Timestamp) * ts2time
			if pkt.Time > 0 {
				pkt.Duration = newTime - pkt.Time
			}
			pkt.Time = newTime

			ready, buf, _ := c.muxer.WritePacket(pkt, false)
			if ready {
				c.send += len(buf)
				c.Fire(buf)
			}

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
	return `video/mp4; codecs="` + c.mimeType + `"`
}

func (c *Consumer) Init() ([]byte, error) {
	c.muxer = mp4f.NewMuxer(nil)
	if err := c.muxer.WriteHeader(c.streams); err != nil {
		return nil, err
	}
	_, data := c.muxer.GetInit(c.streams)
	return data, nil
}

func (c *Consumer) Start() {
	c.start = true
}
