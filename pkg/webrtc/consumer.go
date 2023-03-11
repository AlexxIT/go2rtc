package webrtc

import (
	"encoding/json"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
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
			return c.addConsumerSendTrack(track)

		case streamer.DirectionRecvonly:
			// receive track from WebRTC consumer (microphone, backchannel, two way audio)
			return c.addConsumerRecvTrack(track)
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

func (c *Conn) addConsumerSendTrack(track *streamer.Track) *streamer.Track {
	codec := track.Codec

	// webrtc.codecParametersFuzzySearch
	caps := webrtc.RTPCodecCapability{
		MimeType:  MimeType(codec),
		Channels:  codec.Channels,
		ClockRate: codec.ClockRate,
	}

	if codec.Name == streamer.CodecH264 {
		// don't know if this really neccessary
		// I have tested multiple browsers and H264 profile has no effect on anything
		caps.SDPFmtpLine = "packetization-mode=1;profile-level-id=42e01f"
	}

	// important to use same streamID so JS will automatically
	// join two tracks as one source/stream
	trackLocal, err := webrtc.NewTrackLocalStaticRTP(
		caps, caps.MimeType[:5], "go2rtc",
	)
	if err != nil {
		return nil
	}

	init := webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionSendonly}
	tr, err := c.pc.AddTransceiverFromTrack(trackLocal, init)
	if err != nil {
		return nil
	}

	codecs := []webrtc.RTPCodecParameters{{RTPCodecCapability: caps}}
	if err = tr.SetCodecPreferences(codecs); err != nil {
		return nil
	}

	push := func(packet *rtp.Packet) error {
		c.send += packet.MarshalSize()
		return trackLocal.WriteRTP(packet)
	}

	switch codec.Name {
	case streamer.CodecH264:
		wrapper := h264.RTPPay(1200)
		push = wrapper(push)

		if codec.IsRTP() {
			wrapper = h264.RTPDepay(track)
		} else {
			wrapper = h264.RepairAVC(track)
		}
		push = wrapper(push)

	case streamer.CodecH265:
		// SafariPay because it is the only browser in the world
		// that supports WebRTC + H265
		wrapper := h265.SafariPay(1200)
		push = wrapper(push)

		wrapper = h265.RTPDepay(track)
		push = wrapper(push)
	}

	track = track.Bind(push)
	c.tracks = append(c.tracks, track)
	return track
}

func (c *Conn) addConsumerRecvTrack(track *streamer.Track) *streamer.Track {
	caps := webrtc.RTPCodecCapability{
		MimeType:  MimeType(track.Codec),
		ClockRate: track.Codec.ClockRate,
		Channels:  track.Codec.Channels,
	}

	init := webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}
	tr, err := c.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, init)
	if err != nil {
		return nil
	}

	codecs := []webrtc.RTPCodecParameters{
		{RTPCodecCapability: caps, PayloadType: webrtc.PayloadType(track.Codec.PayloadType)},
	}
	if err = tr.SetCodecPreferences(codecs); err != nil {
		return nil
	}

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
