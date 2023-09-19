package rtsp

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	"github.com/pion/rtp"
)

func (c *Conn) GetMedias() []*core.Media {
	//core.Assert(c.Medias != nil)
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

	if c.mode == core.ModeActiveProducer && track.Codec.Name == core.CodecPCMA {
		// Fix Reolink Doorbell https://github.com/AlexxIT/go2rtc/issues/331
		sender.Handler = pcm.RepackG711(true, sender.Handler)
	}

	sender.HandleRTP(track)

	c.senders = append(c.senders, sender)
	return nil
}

func (c *Conn) packetWriter(codec *core.Codec, channel, payloadType uint8) core.HandlerFunc {
	var buf []byte
	var n int

	video := codec.IsVideo()
	if video {
		buf = make([]byte, 32*1024) // 32KB
	} else {
		buf = make([]byte, 2*1024) // 2KB
	}

	handlerFunc := func(packet *rtp.Packet) {
		if c.state == StateNone || !c.playOK {
			return
		}

		clone := rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         packet.Marker,
				PayloadType:    payloadType,
				SequenceNumber: packet.SequenceNumber,
				Timestamp:      packet.Timestamp,
				SSRC:           packet.SSRC,
			},
			Payload: packet.Payload,
		}

		if !video {
			packet.Marker = true // better to have marker on all audio packets
		}

		size := 12 + len(packet.Payload)

		if n+4+size > len(buf) {
			if len(buf) < 1024*1024 {
				buf = append(buf, make([]byte, len(buf))...)
			} else {
				if _, err := c.conn.Write(buf[:n]); err == nil {
					c.send += n
				}
				n = 0
			}
		}

		//log.Printf("[RTP] codec: %s, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, mark: %v", codec.Name, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

		chunk := buf[n:]
		_ = chunk[4] // bounds
		chunk[0] = '$'
		chunk[1] = channel
		chunk[2] = byte(size >> 8)
		chunk[3] = byte(size)

		if _, err := clone.MarshalTo(chunk[4:]); err != nil {
			return
		}

		n += 4 + size

		if !packet.Marker {
			return // collect continious video packets to buffer
		}

		if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
			return
		}

		//log.Printf("[rtsp] channel:%2d write_size:%6d buffer_size:%6d", channel, n, len(buf))

		if _, err := c.conn.Write(buf[:n]); err == nil {
			c.send += n
		}

		n = 0
	}

	if !codec.IsRTP() {
		switch codec.Name {
		case core.CodecH264:
			handlerFunc = h264.RTPPay(c.PacketSize, handlerFunc)
		case core.CodecH265:
			handlerFunc = h265.RTPPay(c.PacketSize, handlerFunc)
		case core.CodecAAC:
			handlerFunc = aac.RTPPay(handlerFunc)
		case core.CodecJPEG:
			handlerFunc = mjpeg.RTPPay(handlerFunc)
		}
	} else if c.PacketSize != 0 {
		switch codec.Name {
		case core.CodecH264:
			handlerFunc = h264.RTPPay(c.PacketSize, handlerFunc)
			handlerFunc = h264.RTPDepay(codec, handlerFunc)
		case core.CodecH265:
			handlerFunc = h265.RTPPay(c.PacketSize, handlerFunc)
			handlerFunc = h265.RTPDepay(codec, handlerFunc)
		}
	}

	return handlerFunc
}
