package rtsp

import (
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/pion/rtp"
	"time"
)

func (c *Conn) GetMedias() []*core.Media {
	core.Assert(c.Medias != nil)
	return c.Medias
}

func (c *Conn) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) (err error) {
	core.Assert(media.Direction == core.DirectionSendonly)

	for _, sender := range c.senders {
		if sender.Codec == codec {
			sender.HandleRTP(track)
			return
		}
	}

	var channel byte

	switch c.mode {
	case core.ModeActiveProducer: // backchannel
		c.stateMu.Lock()
		defer c.stateMu.Unlock()

		if c.state == StatePlay {
			if err = c.Reconnect(); err != nil {
				return
			}
		}

		if channel, err = c.SetupMedia(media); err != nil {
			return
		}

		c.state = StateSetup

	case core.ModePassiveConsumer:
		channel = byte(len(c.senders)) * 2

		// for consumer is better to use original track codec
		codec = track.Codec.Clone()
		// generate new payload type, starting from 96
		codec.PayloadType = byte(96 + len(c.senders))

	default:
		panic(core.Caller())
	}

	// save original codec to sender (can have Codec.Name = ANY)
	sender := core.NewSender(media, codec)
	// important to send original codec for valid IsRTP check
	sender.Handler = c.packetWriter(track.Codec, channel, codec.PayloadType)
	sender.HandleRTP(track)

	c.senders = append(c.senders, sender)
	return nil
}

func (c *Conn) packetWriter(codec *core.Codec, channel, payloadType uint8) core.HandlerFunc {
	handlerFunc := func(packet *rtp.Packet) {
		if c.state == StateNone {
			return
		}

		clone := *packet
		clone.Header.PayloadType = payloadType

		size := clone.MarshalSize()

		//log.Printf("[RTP] codec: %s, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, mark: %v", codec.Name, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

		data := make([]byte, 4+size)
		data[0] = '$'
		data[1] = channel
		data[2] = byte(size >> 8)
		data[3] = byte(size)

		if _, err := clone.MarshalTo(data[4:]); err != nil {
			return
		}

		if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
			return
		}

		n, err := c.conn.Write(data)
		if err != nil {
			return
		}

		c.send += n
	}

	if !codec.IsRTP() {
		switch codec.Name {
		case core.CodecH264:
			handlerFunc = h264.RTPPay(1500, handlerFunc)
		case core.CodecH265:
			handlerFunc = h265.RTPPay(1500, handlerFunc)
		case core.CodecAAC:
			handlerFunc = aac.RTPPay(handlerFunc)
		case core.CodecJPEG:
			handlerFunc = mjpeg.RTPPay(handlerFunc)
		}
	}

	return handlerFunc
}
