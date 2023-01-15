package homekit

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/rtp"
	"net"
	"net/url"
	"sync/atomic"
)

type Client struct {
	streamer.Element

	conn   *hap.Conn
	exit   chan error
	server *srtp.Server
	url    string

	medias []*streamer.Media
	tracks []*streamer.Track

	sessions []*srtp.Session
}

func NewClient(rawURL string, server *srtp.Server) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	c := &hap.Conn{
		DeviceAddress: u.Host,
		DeviceID:      query.Get("device_id"),
		DevicePublic:  hap.DecodeKey(query.Get("device_public")),
		ClientID:      query.Get("client_id"),
		ClientPrivate: hap.DecodeKey(query.Get("client_private")),
	}

	return &Client{conn: c, server: server}, nil
}

func (c *Client) Dial() error {
	if err := c.conn.Dial(); err != nil {
		return err
	}

	c.exit = make(chan error)

	go func() {
		//start goroutine for reading responses from camera
		c.exit <- c.conn.Handle()
	}()

	return nil
}

func (c *Client) GetMedias() []*streamer.Media {
	if c.medias == nil {
		c.medias = c.getMedias()
	}

	return c.medias
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	for _, track := range c.tracks {
		if track.Codec == codec {
			return track
		}
	}

	track := streamer.NewTrack(codec, media.Direction)
	c.tracks = append(c.tracks, track)
	return track
}

func (c *Client) Start() error {
	if c.tracks == nil {
		return errors.New("producer without tracks")
	}

	// get our server local IP-address
	host, _, err := net.SplitHostPort(c.conn.LocalAddr())
	if err != nil {
		return err
	}

	// TODO: set right config
	vp := &rtp.VideoParameters{
		CodecType: rtp.VideoCodecType_H264,
		CodecParams: rtp.VideoCodecParameters{
			Profiles: []rtp.VideoCodecProfile{
				{Id: rtp.VideoCodecProfileMain},
			},
			Levels: []rtp.VideoCodecLevel{
				{Level: rtp.VideoCodecLevel4},
			},
			Packetizations: []rtp.VideoCodecPacketization{
				{Mode: rtp.VideoCodecPacketizationModeNonInterleaved},
			},
		},
		Attributes: rtp.VideoCodecAttributes{
			Width: 1920, Height: 1080, Framerate: 30,
		},
	}

	ap := &rtp.AudioParameters{
		CodecType: rtp.AudioCodecType_AAC_ELD,
		CodecParams: rtp.AudioCodecParameters{
			Channels:   1,
			Bitrate:    rtp.AudioCodecBitrateVariable,
			Samplerate: rtp.AudioCodecSampleRate16Khz,
			// packet time=20 => AAC-ELD packet size=480
			// packet time=30 => AAC-ELD packet size=480
			// packet time=40 => AAC-ELD packet size=480
			// packet time=60 => AAC-LD  packet size=960
			PacketTime: 40,
		},
	}

	// setup HomeKit stream session
	hkSession := camera.NewSession(vp, ap)
	hkSession.SetLocalEndpoint(host, c.server.Port())

	// create client for processing camera accessory
	cam := camera.NewClient(c.conn)
	// try to start HomeKit stream
	if err = cam.StartStream(hkSession); err != nil {
		return err
	}

	// SRTP Video Session
	vs := &srtp.Session{
		LocalSSRC:  hkSession.Config.Video.RTP.Ssrc,
		RemoteSSRC: hkSession.Answer.SsrcVideo,
	}
	if err = vs.SetKeys(
		hkSession.Offer.Video.MasterKey, hkSession.Offer.Video.MasterSalt,
		hkSession.Answer.Video.MasterKey, hkSession.Answer.Video.MasterSalt,
	); err != nil {
		return err
	}

	// SRTP Audio Session
	as := &srtp.Session{
		LocalSSRC:  hkSession.Config.Audio.RTP.Ssrc,
		RemoteSSRC: hkSession.Answer.SsrcAudio,
	}
	if err = as.SetKeys(
		hkSession.Offer.Audio.MasterKey, hkSession.Offer.Audio.MasterSalt,
		hkSession.Answer.Audio.MasterKey, hkSession.Answer.Audio.MasterSalt,
	); err != nil {
		return err
	}

	for _, track := range c.tracks {
		switch track.Codec.Name {
		case streamer.CodecH264:
			vs.Track = track
		case streamer.CodecELD:
			as.Track = track
		}
	}

	c.server.AddSession(vs)
	c.server.AddSession(as)

	c.sessions = []*srtp.Session{vs, as}

	return <-c.exit
}

func (c *Client) Stop() error {
	err := c.conn.Close()

	for _, session := range c.sessions {
		c.server.RemoveSession(session)
	}

	return err
}

func (c *Client) getMedias() []*streamer.Media {
	var medias []*streamer.Media

	accs, err := c.conn.GetAccessories()
	if err != nil {
		return nil
	}

	acc := accs[0]

	// get supported video config (not really necessary)
	char := acc.GetCharacter(characteristic.TypeSupportedVideoStreamConfiguration)
	v1 := &rtp.VideoStreamConfiguration{}
	if err = char.ReadTLV8(v1); err != nil {
		return nil
	}

	for _, hkCodec := range v1.Codecs {
		codec := &streamer.Codec{ClockRate: 90000}

		switch hkCodec.Type {
		case rtp.VideoCodecType_H264:
			codec.Name = streamer.CodecH264
			codec.FmtpLine = "profile-level-id=420029"
		default:
			fmt.Printf("unknown codec: %d", hkCodec.Type)
			continue
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
		return nil
	}

	for _, hkCodec := range v2.Codecs {
		codec := &streamer.Codec{
			Channels: uint16(hkCodec.Parameters.Channels),
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

		switch hkCodec.Type {
		case rtp.AudioCodecType_AAC_ELD:
			codec.Name = streamer.CodecELD
			// only this value supported by FFmpeg
			codec.FmtpLine = "profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=F8EC3000"
		default:
			fmt.Printf("unknown codec: %d", hkCodec.Type)
			continue
		}

		media := &streamer.Media{
			Kind: streamer.KindAudio, Direction: streamer.DirectionSendonly,
			Codecs: []*streamer.Codec{codec},
		}
		medias = append(medias, media)
	}

	return medias
}

func (c *Client) MarshalJSON() ([]byte, error) {
	var recv uint32
	for _, session := range c.sessions {
		recv += atomic.LoadUint32(&session.Recv)
	}

	info := &streamer.Info{
		Type:   "HomeKit source",
		URL:    c.conn.URL(),
		Medias: c.medias,
		Tracks: c.tracks,
		Recv:   recv,
	}
	return json.Marshal(info)
}
