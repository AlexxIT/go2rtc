package webrtc

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

func (c *Conn) GetMedias() []*streamer.Media {
	return c.medias
}

func (c *Conn) AddTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	switch c.Mode {
	case streamer.ModePassiveConsumer:
		switch track.Direction {
		case streamer.DirectionSendonly:
			// send our track to WebRTC consumer
			return c.addSendTrack(media, track)

		case streamer.DirectionRecvonly:
			// receive track from WebRTC consumer (microphone, backchannel, two way audio)
			return c.addConsumerRecvTrack(media, track)
		}

	case streamer.ModePassiveProducer:
		// "Stream to camera" function
		consCodec := media.MatchCodec(track.Codec)
		consTrack := c.GetTrack(media, consCodec)
		if consTrack == nil {
			return nil
		}

		return track.Bind(func(packet *rtp.Packet) error {
			return consTrack.WriteRTP(packet)
		})
	}

	panic("not implemented")
}

func (c *Conn) addConsumerRecvTrack(media *streamer.Media, track *streamer.Track) *streamer.Track {
	params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  MimeType(track.Codec),
			ClockRate: track.Codec.ClockRate,
			Channels:  track.Codec.Channels,
		},
		PayloadType: 0, // don't know if this necessary
	}

	tr := c.getTranseiver(media.MID)

	// set codec for consumer recv track so remote peer should send media with this codec
	_ = tr.SetCodecPreferences([]webrtc.RTPCodecParameters{params})

	c.tracks = append(c.tracks, track)
	return track
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	info := &streamer.Info{
		Type:       c.Desc + " " + c.Mode.String(),
		RemoteAddr: c.remote,
		UserAgent:  c.UserAgent,
		Medias:     c.medias,
		Tracks:     c.tracks,
		Recv:       uint32(c.receive),
		Send:       uint32(c.send),
	}
	return json.Marshal(info)
}
