package mpegts

import (
	"bytes"
	"encoding/hex"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/ts"
	"github.com/pion/rtp"
	"sync/atomic"
	"time"
)

type Consumer struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	buf      *bytes.Buffer
	muxer    *ts.Muxer
	mimeType string
	streams  []av.CodecData
	start    bool
	init     []byte

	send uint32
}

func (c *Consumer) GetMedias() []*streamer.Media {
	return []*streamer.Media{
		{
			Kind:      streamer.KindVideo,
			Direction: streamer.DirectionRecvonly,
			Codecs: []*streamer.Codec{
				{Name: streamer.CodecH264},
			},
		},
		//{
		//	Kind:      streamer.KindAudio,
		//	Direction: streamer.DirectionRecvonly,
		//	Codecs: []*streamer.Codec{
		//		{Name: streamer.CodecAAC},
		//	},
		//},
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

		if len(c.mimeType) > 0 {
			c.mimeType += ","
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

			if err = c.muxer.WritePacket(pkt); err != nil {
				return err
			}

			// clone bytes from buffer, so next packet won't overwrite it
			buf := append([]byte{}, c.buf.Bytes()...)
			atomic.AddUint32(&c.send, uint32(len(buf)))
			c.Fire(buf)

			c.buf.Reset()

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

		if len(c.mimeType) > 0 {
			c.mimeType += ","
		}

		c.mimeType += "mp4a.40.2"
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

			if err := c.muxer.WritePacket(pkt); err != nil {
				return err
			}

			// clone bytes from buffer, so next packet won't overwrite it
			buf := append([]byte{}, c.buf.Bytes()...)
			atomic.AddUint32(&c.send, uint32(len(buf)))
			c.Fire(buf)

			c.buf.Reset()

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

func (c *Consumer) MimeCodecs() string {
	return c.mimeType
}

func (c *Consumer) Init() ([]byte, error) {
	c.buf = bytes.NewBuffer(nil)
	c.muxer = ts.NewMuxer(c.buf)

	// first packet will be with header, it's ok
	if err := c.muxer.WriteHeader(c.streams); err != nil {
		return nil, err
	}
	data := append([]byte{}, c.buf.Bytes()...)

	return data, nil
}

func (c *Consumer) Start() {
	c.start = true
}
