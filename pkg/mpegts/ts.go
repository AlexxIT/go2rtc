package mpegts

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/ts"
	"github.com/pion/rtp"
	"time"
)

type Consumer struct {
	core.Listener

	UserAgent  string
	RemoteAddr string

	senders []*core.Sender

	buf      *bytes.Buffer
	muxer    *ts.Muxer
	mimeType string
	streams  []av.CodecData
	start    bool
	init     []byte

	send int
}

func (c *Consumer) GetMedias() []*core.Media {
	return []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
			},
		},
		//{
		//	Kind:      core.KindAudio,
		//	Direction: core.DirectionSendonly,
		//	Codecs: []*core.Codec{
		//		{Name: core.CodecAAC},
		//	},
		//},
	}
}

func (c *Consumer) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	trackID := int8(len(c.streams))

	handler := core.NewSender(media, track.Codec)

	switch track.Codec.Name {
	case core.CodecH264:
		sps, pps := h264.GetParameterSet(track.Codec.FmtpLine)
		stream, err := h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
		if err != nil {
			return nil
		}

		if len(c.mimeType) > 0 {
			c.mimeType += ","
		}

		c.mimeType += "avc1." + h264.GetProfileLevelID(track.Codec.FmtpLine)

		c.streams = append(c.streams, stream)

		pkt := av.Packet{Idx: trackID, CompositionTime: time.Millisecond}

		ts2time := time.Second / time.Duration(track.Codec.ClockRate)

		handler.Handler = func(packet *rtp.Packet) {
			if packet.Version != h264.RTPPacketVersionAVC {
				return
			}

			if !c.start {
				return
			}

			pkt.Data = packet.Payload
			newTime := time.Duration(packet.Timestamp) * ts2time
			if pkt.Time > 0 {
				pkt.Duration = newTime - pkt.Time
			}
			pkt.Time = newTime

			if err = c.muxer.WritePacket(pkt); err != nil {
				return
			}

			// clone bytes from buffer, so next packet won't overwrite it
			buf := append([]byte{}, c.buf.Bytes()...)
			c.Fire(buf)

			c.send += len(buf)

			c.buf.Reset()
		}

		if track.Codec.IsRTP() {
			handler.Handler = h264.RTPDepay(track.Codec, handler.Handler)
		} else {
			handler.Handler = h264.RepairAVC(track.Codec, handler.Handler)
		}

	case core.CodecAAC:
		s := core.Between(track.Codec.FmtpLine, "config=", ";")

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

		ts2time := time.Second / time.Duration(track.Codec.ClockRate)

		handler.Handler = func(packet *rtp.Packet) {
			if !c.start {
				return
			}

			pkt.Data = packet.Payload
			newTime := time.Duration(packet.Timestamp) * ts2time
			if pkt.Time > 0 {
				pkt.Duration = newTime - pkt.Time
			}
			pkt.Time = newTime

			if err = c.muxer.WritePacket(pkt); err != nil {
				return
			}

			// clone bytes from buffer, so next packet won't overwrite it
			buf := append([]byte{}, c.buf.Bytes()...)
			c.Fire(buf)

			c.send += len(buf)

			c.buf.Reset()
		}

		if track.Codec.IsRTP() {
			handler.Handler = aac.RTPDepay(handler.Handler)
		}

	default:
		panic("unsupported codec")
	}

	handler.HandleRTP(track)
	c.senders = append(c.senders, handler)

	return nil
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

func (c *Consumer) Stop() error {
	for _, sender := range c.senders {
		sender.Close()
	}
	return nil
}

func (c *Consumer) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:       "TS passive consumer",
		RemoteAddr: c.RemoteAddr,
		UserAgent:  c.UserAgent,
		Medias:     c.GetMedias(),
		Senders:    c.senders,
		Send:       c.send,
	}
	return json.Marshal(info)
}
