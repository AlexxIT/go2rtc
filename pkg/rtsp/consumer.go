package rtsp

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mjpeg"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

func (c *Conn) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	switch c.mode {
	// send our track to RTSP consumer (ex. FFmpeg)
	case ModeServerConsumer:
		i := len(c.tracks)
		channelID := byte(i << 1)

		codec := track.Codec.Clone()
		codec.PayloadType = uint8(96 + i)

		if media.MatchAll() {
			// fill consumer medias list
			c.Medias = append(c.Medias, &streamer.Media{
				Kind: media.Kind, Direction: media.Direction,
				Codecs: []*streamer.Codec{codec},
			})
		} else {
			// find consumer media and replace codec with right one
			for i, m := range c.Medias {
				if m == media {
					media.Codecs = []*streamer.Codec{codec}
					c.Medias[i] = media
					break
				}
			}
		}

		track = c.bindTrack(track, channelID, codec.PayloadType)
		track.Codec = codec
		c.tracks = append(c.tracks, track)

		return track

	// camera with backchannel support
	case ModeClientProducer:
		consCodec := media.MatchCodec(track.Codec)
		consTrack := c.GetTrack(media, consCodec)
		if consTrack == nil {
			return nil
		}

		return track.Bind(func(packet *rtp.Packet) error {
			return consTrack.WriteRTP(packet)
		})
	}

	println("WARNING: rtsp: AddTrack to wrong mode")
	return nil
}

func (c *Conn) bindTrack(
	track *streamer.Track, channel uint8, payloadType uint8,
) *streamer.Track {
	push := func(packet *rtp.Packet) error {
		if c.state == StateNone {
			return nil
		}
		packet.Header.PayloadType = payloadType

		size := packet.MarshalSize()

		//log.Printf("[RTP] codec: %s, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, mark: %v", track.Codec.Name, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

		data := make([]byte, 4+size)
		data[0] = '$'
		data[1] = channel
		binary.BigEndian.PutUint16(data[2:], uint16(size))

		if _, err := packet.MarshalTo(data[4:]); err != nil {
			return nil
		}

		if _, err := c.conn.Write(data); err != nil {
			return err
		}

		c.send += size

		return nil
	}

	if !track.Codec.IsRTP() {
		switch track.Codec.Name {
		case streamer.CodecH264:
			wrapper := h264.RTPPay(1500)
			push = wrapper(push)
		case streamer.CodecH265:
			wrapper := h265.RTPPay(1500)
			push = wrapper(push)
		case streamer.CodecAAC:
			wrapper := aac.RTPPay(1500)
			push = wrapper(push)
		case streamer.CodecJPEG:
			wrapper := mjpeg.RTPPay()
			push = wrapper(push)
		}
	}

	return track.Bind(push)
}
