package homekit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/pion/rtp"
)

type Client struct {
	core.Listener

	conn   *hap.Client
	server *srtp.Server
	config *StreamConfig

	medias    []*core.Media
	receivers []*core.Receiver

	sessions []*srtp.Session
}

type StreamConfig struct {
	Video camera.SupportedVideoStreamConfig
	Audio camera.SupportedAudioStreamConfig
}

func NewClient(rawURL string, server *srtp.Server) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	c := &hap.Client{
		DeviceAddress: u.Host,
		DeviceID:      query.Get("device_id"),
		DevicePublic:  hap.DecodeKey(query.Get("device_public")),
		ClientID:      query.Get("client_id"),
		ClientPrivate: hap.DecodeKey(query.Get("client_private")),
	}

	return &Client{conn: c, server: server}, nil
}

func (c *Client) Dial() error {
	return c.conn.Dial()
}

func (c *Client) GetMedias() []*core.Media {
	if c.medias != nil {
		return c.medias
	}

	accs, err := c.conn.GetAccessories()
	if err != nil {
		return nil
	}

	acc := accs[0]

	c.config = &StreamConfig{}

	// get supported video config (not really necessary)
	char := acc.GetCharacter(camera.TypeSupportedVideoStreamConfiguration)
	if char == nil {
		return nil
	}
	if err = char.ReadTLV8(&c.config.Video); err != nil {
		return nil
	}

	for _, videoCodec := range c.config.Video.Codecs {
		var name string

		switch videoCodec.CodecType {
		case camera.VideoCodecTypeH264:
			name = core.CodecH264
		default:
			continue
		}

		for _, params := range videoCodec.CodecParams {
			codec := &core.Codec{
				Name:      name,
				ClockRate: 90000,
				FmtpLine:  "profile-level-id=",
			}

			switch params.ProfileID {
			case camera.VideoCodecProfileConstrainedBaseline:
				codec.FmtpLine += "4200" // 4240?
			case camera.VideoCodecProfileMain:
				codec.FmtpLine += "4D00" // 4D40?
			case camera.VideoCodecProfileHigh:
				codec.FmtpLine += "6400"
			default:
				continue
			}

			switch params.Level {
			case camera.VideoCodecLevel31:
				codec.FmtpLine += "1F"
			case camera.VideoCodecLevel32:
				codec.FmtpLine += "20"
			case camera.VideoCodecLevel40:
				codec.FmtpLine += "28"
			default:
				continue
			}

			media := &core.Media{
				Kind: core.KindVideo, Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)
		}
	}

	char = acc.GetCharacter(camera.TypeSupportedAudioStreamConfiguration)
	if char == nil {
		return nil
	}
	if err = char.ReadTLV8(&c.config.Audio); err != nil {
		return nil
	}

	for _, audioCodec := range c.config.Audio.Codecs {
		var name string

		switch audioCodec.CodecType {
		case camera.AudioCodecTypePCMU:
			name = core.CodecPCMU
		case camera.AudioCodecTypePCMA:
			name = core.CodecPCMA
		case camera.AudioCodecTypeAACELD:
			name = core.CodecELD
		case camera.AudioCodecTypeOpus:
			name = core.CodecOpus
		default:
			continue
		}

		for _, params := range audioCodec.CodecParams {
			codec := &core.Codec{
				Name:     name,
				Channels: uint16(params.Channels),
			}

			if name == core.CodecELD {
				// only this value supported by FFmpeg
				codec.FmtpLine = "profile-level-id=1;mode=AAC-hbr;sizelength=13;indexlength=3;indexdeltalength=3;config=F8EC3000"
			}

			switch params.SampleRate {
			case camera.AudioCodecSampleRate8Khz:
				codec.ClockRate = 8000
			case camera.AudioCodecSampleRate16Khz:
				codec.ClockRate = 16000
			case camera.AudioCodecSampleRate24Khz:
				codec.ClockRate = 24000
			default:
				continue
			}

			media := &core.Media{
				Kind: core.KindAudio, Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)
		}
	}

	media := &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
		Codecs: []*core.Codec{
			{
				Name:        core.CodecJPEG,
				ClockRate:   90000,
				PayloadType: core.PayloadTypeRAW,
			},
		},
	}
	c.medias = append(c.medias, media)

	return c.medias
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	for _, track := range c.receivers {
		if track.Codec == codec {
			return track, nil
		}
	}

	track := core.NewReceiver(media, codec)
	c.receivers = append(c.receivers, track)
	return track, nil
}

func (c *Client) Start() error {
	if c.receivers == nil {
		return errors.New("producer without tracks")
	}

	if c.receivers[0].Codec.Name == core.CodecJPEG {
		return c.startMJPEG()
	}

	// get our server local IP-address
	host, _, err := net.SplitHostPort(c.conn.LocalAddr())
	if err != nil {
		return err
	}

	videoParams := &camera.SelectedVideoParams{
		CodecType: camera.VideoCodecTypeH264,
		VideoAttrs: camera.VideoAttrs{
			Width: 1920, Height: 1080, Framerate: 30,
		},
	}

	videoTrack := c.trackByKind(core.KindVideo)
	if videoTrack != nil {
		profile := h264.GetProfileLevelID(videoTrack.Codec.FmtpLine)

		switch profile[:2] {
		case "42":
			videoParams.CodecParams.ProfileID = camera.VideoCodecProfileConstrainedBaseline
		case "4D":
			videoParams.CodecParams.ProfileID = camera.VideoCodecProfileMain
		case "64":
			videoParams.CodecParams.ProfileID = camera.VideoCodecProfileHigh
		}

		switch profile[4:] {
		case "1F":
			videoParams.CodecParams.Level = camera.VideoCodecLevel31
		case "20":
			videoParams.CodecParams.Level = camera.VideoCodecLevel32
		case "28":
			videoParams.CodecParams.Level = camera.VideoCodecLevel40
		}
	} else {
		// if consumer don't need track - ask first track from camera
		codec0 := c.config.Video.Codecs[0]
		videoParams.CodecParams.ProfileID = codec0.CodecParams[0].ProfileID
		videoParams.CodecParams.Level = codec0.CodecParams[0].Level
	}

	audioParams := &camera.SelectedAudioParams{
		CodecParams: camera.AudioCodecParams{
			Bitrate: camera.AudioCodecBitrateVariable,
			// RTPTime=20 => AAC-ELD packet size=480
			// RTPTime=30 => AAC-ELD packet size=480
			// RTPTime=40 => AAC-ELD packet size=480
			// RTPTime=60 => AAC-LD  packet size=960
			RTPTime: 40,
		},
	}

	audioTrack := c.trackByKind(core.KindAudio)
	if audioTrack != nil {
		audioParams.CodecParams.Channels = byte(audioTrack.Codec.Channels)

		switch audioTrack.Codec.Name {
		case core.CodecPCMU:
			audioParams.CodecType = camera.AudioCodecTypePCMU
		case core.CodecPCMA:
			audioParams.CodecType = camera.AudioCodecTypePCMA
		case core.CodecELD:
			audioParams.CodecType = camera.AudioCodecTypeAACELD
		case core.CodecOpus:
			audioParams.CodecType = camera.AudioCodecTypeOpus
		}

		switch audioTrack.Codec.ClockRate {
		case 8000:
			audioParams.CodecParams.SampleRate = camera.AudioCodecSampleRate8Khz
		case 16000:
			audioParams.CodecParams.SampleRate = camera.AudioCodecSampleRate16Khz
		case 24000:
			audioParams.CodecParams.SampleRate = camera.AudioCodecSampleRate24Khz
		}
	} else {
		// if consumer don't need track - ask first track from camera
		codec0 := c.config.Audio.Codecs[0]
		audioParams.CodecType = codec0.CodecType
		audioParams.CodecParams.Channels = codec0.CodecParams[0].Channels
		audioParams.CodecParams.SampleRate = codec0.CodecParams[0].SampleRate
	}

	// setup HomeKit stream session
	session := camera.NewSession(videoParams, audioParams)
	session.SetLocalEndpoint(host, c.server.Port())

	// create client for processing camera accessory
	cam := camera.NewClient(c.conn)
	// try to start HomeKit stream
	if err = cam.StartStream(session); err != nil {
		return err
	}

	// SRTP Video Session
	videoSession := &srtp.Session{
		LocalSSRC:  session.Config.VideoParams.RTPParams.SSRC,
		RemoteSSRC: session.Answer.VideoSSRC,
		Track:      videoTrack,
	}
	if err = videoSession.SetKeys(
		session.Offer.VideoCrypto.MasterKey, session.Offer.VideoCrypto.MasterSalt,
		session.Answer.VideoCrypto.MasterKey, session.Answer.VideoCrypto.MasterSalt,
	); err != nil {
		return err
	}

	// SRTP Audio Session
	audioSession := &srtp.Session{
		LocalSSRC:  session.Config.AudioParams.RTPParams.SSRC,
		RemoteSSRC: session.Answer.AudioSSRC,
		Track:      audioTrack,
	}
	if err = audioSession.SetKeys(
		session.Offer.AudioCrypto.MasterKey, session.Offer.AudioCrypto.MasterSalt,
		session.Answer.AudioCrypto.MasterKey, session.Answer.AudioCrypto.MasterSalt,
	); err != nil {
		return err
	}

	c.server.AddSession(videoSession)
	c.server.AddSession(audioSession)

	c.sessions = []*srtp.Session{videoSession, audioSession}

	if audioSession.Track != nil {
		audioSession.Deadline = time.NewTimer(core.ConnDeadline)
		<-audioSession.Deadline.C
	} else if videoSession.Track != nil {
		videoSession.Deadline = time.NewTimer(core.ConnDeadline)
		<-videoSession.Deadline.C
	}

	return nil
}

func (c *Client) Stop() error {
	for _, session := range c.sessions {
		c.server.RemoveSession(session)
	}

	return c.conn.Close()
}

func (c *Client) MarshalJSON() ([]byte, error) {
	var recv uint32
	for _, session := range c.sessions {
		recv += atomic.LoadUint32(&session.Recv)
	}

	info := &core.Info{
		Type:      "HomeKit active producer",
		URL:       c.conn.URL(),
		SDP:       fmt.Sprintf("%+v", *c.config),
		Medias:    c.medias,
		Receivers: c.receivers,
		Recv:      int(recv),
	}
	return json.Marshal(info)
}

func (c *Client) trackByKind(kind string) *core.Receiver {
	for _, receiver := range c.receivers {
		if core.GetKind(receiver.Codec.Name) == kind {
			return receiver
		}
	}
	return nil
}

func (c *Client) startMJPEG() error {
	receiver := c.receivers[0]

	for {
		b, err := c.conn.GetImage(1920, 1080)
		if err != nil {
			return err
		}

		packet := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: b,
		}
		receiver.WriteRTP(packet)
	}
}
