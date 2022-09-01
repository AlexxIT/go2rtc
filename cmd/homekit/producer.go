package homekit

import (
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/cmd/srtp"
	"github.com/AlexxIT/go2rtc/pkg/homekit"
	"github.com/AlexxIT/go2rtc/pkg/homekit/camera"
	pkg "github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/rtp"
	"net"
	"strconv"
)

type Producer struct {
	streamer.Element

	client *homekit.Client
	medias []*streamer.Media
	tracks []*streamer.Track

	sessions []*pkg.Session
}

func (c *Producer) GetMedias() []*streamer.Media {
	if c.medias == nil {
		c.medias = c.getMedias()
	}

	return c.medias
}

func (c *Producer) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}

	track := &streamer.Track{Codec: codec, Direction: media.Direction}
	c.tracks = append(c.tracks, track)
	return track
}

func (c *Producer) Start() error {
	if c.tracks == nil {
		return errors.New("producer without tracks")
	}

	// get our server local IP-address
	host, _, err := net.SplitHostPort(c.client.LocalAddr())
	if err != nil {
		return err
	}

	// get our server SRTP port
	port, err := strconv.Atoi(srtp.Port)
	if err != nil {
		return err
	}

	// setup HomeKit stream session
	hkSession := camera.NewSession()
	hkSession.SetLocalEndpoint(host, uint16(port))

	// create client for processing camera accessory
	cam := camera.NewClient(c.client)
	// try to start HomeKit stream
	if err = cam.StartStream2(hkSession); err != nil {
		panic(err) // TODO: fixme
	}

	// SRTP Video Session
	vs := &pkg.Session{
		LocalSSRC:  hkSession.Config.Video.RTP.Ssrc,
		RemoteSSRC: hkSession.Answer.SsrcVideo,
		Track:      c.tracks[0],
	}
	if err = vs.SetKeys(
		hkSession.Offer.Video.MasterKey, hkSession.Offer.Video.MasterSalt,
		hkSession.Answer.Video.MasterKey, hkSession.Answer.Video.MasterSalt,
	); err != nil {
		return err
	}

	// SRTP Audio Session
	as := &pkg.Session{
		LocalSSRC:  hkSession.Config.Audio.RTP.Ssrc,
		RemoteSSRC: hkSession.Answer.SsrcAudio,
		Track:      &streamer.Track{},
	}
	if err = as.SetKeys(
		hkSession.Offer.Audio.MasterKey, hkSession.Offer.Audio.MasterSalt,
		hkSession.Answer.Audio.MasterKey, hkSession.Answer.Audio.MasterSalt,
	); err != nil {
		return err
	}

	srtp.AddSession(vs)
	srtp.AddSession(as)

	c.sessions = []*pkg.Session{vs, as}

	return nil
}

func (c *Producer) Stop() error {
	err := c.client.Close()

	for _, session := range c.sessions {
		srtp.RemoveSession(session)
	}

	return err
}

func (c *Producer) getMedias() []*streamer.Media {
	var medias []*streamer.Media

	accs, err := c.client.GetAccessories()
	acc := accs[0]
	if err != nil {
		panic(err)
	}

	// get supported video config (not really necessary)
	char := acc.GetCharacter(characteristic.TypeSupportedVideoStreamConfiguration)
	v1 := &rtp.VideoStreamConfiguration{}
	if err = char.ReadTLV8(v1); err != nil {
		panic(err)
	}

	for _, hkCodec := range v1.Codecs {
		codec := &streamer.Codec{ClockRate: 90000}

		switch hkCodec.Type {
		case rtp.VideoCodecType_H264:
			codec.Name = streamer.CodecH264
		default:
			panic(fmt.Sprintf("unknown codec: %d", hkCodec.Type))
		}

		media := &streamer.Media{
			Kind: streamer.KindVideo, Direction: streamer.DirectionSendonly,
			Codecs: []*streamer.Codec{codec},
		}
		medias = append(medias, media)
	}

	char = acc.GetCharacter(characteristic.TypeSupportedAudioStreamConfiguration)
	v2 := &rtp.AudioStreamConfiguration{}
	if err = char.ReadTLV8(v2); err != nil {
		panic(err)
	}

	for _, hkCodec := range v2.Codecs {
		codec := &streamer.Codec{
			Channels: uint16(hkCodec.Parameters.Channels),
		}

		switch hkCodec.Type {
		case rtp.AudioCodecType_AAC_ELD:
			codec.Name = streamer.CodecAAC
		default:
			panic(fmt.Sprintf("unknown codec: %d", hkCodec.Type))
		}

		switch hkCodec.Parameters.Samplerate {
		case rtp.AudioCodecSampleRate8Khz:
			codec.ClockRate = 8000
		case rtp.AudioCodecSampleRate16Khz:
			codec.ClockRate = 16000
		case rtp.AudioCodecSampleRate24Khz:
			codec.ClockRate = 24000
		default:
			panic(fmt.Sprintf("unknown clockrate: %d", hkCodec.Parameters.Samplerate))
		}

		media := &streamer.Media{
			Kind: streamer.KindAudio, Direction: streamer.DirectionSendonly,
			Codecs: []*streamer.Codec{codec},
		}
		medias = append(medias, media)
	}

	return medias
}
