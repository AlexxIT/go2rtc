package homekit

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/opus"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/pion/rtp"
)

type Consumer struct {
	core.Connection
	conn net.Conn
	srtp *srtp.Server

	deadline *time.Timer

	sessionID    string
	videoSession *srtp.Session
	audioSession *srtp.Session
	audioRTPTime byte

	audioCodec *core.Codec
	backTrack  *core.Receiver // backchannel audio (HomeKit viewer → camera)
	closers    []io.Closer
}

func NewConsumer(conn net.Conn, server *srtp.Server) *Consumer {
	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecH264},
			},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs: []*core.Codec{
				{Name: core.CodecOpus},
			},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{Name: core.CodecOpus},
				{Name: core.CodecPCMA},
				{Name: core.CodecPCMU},
				{Name: core.CodecAAC},
				{Name: core.CodecPCM},
				{Name: core.CodecPCML},
			},
		},
	}
	return &Consumer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "homekit",
			Protocol:   "rtp",
			RemoteAddr: conn.RemoteAddr().String(),
			Medias:     medias,
			Transport:  conn,
		},
		conn: conn,
		srtp: server,
	}
}

func (c *Consumer) SessionID() string {
	return c.sessionID
}

func (c *Consumer) SetOffer(offer *camera.SetupEndpointsRequest) {
	c.sessionID = offer.SessionID
	c.videoSession = &srtp.Session{
		Remote: &srtp.Endpoint{
			Addr:       offer.Address.IPAddr,
			Port:       offer.Address.VideoRTPPort,
			MasterKey:  []byte(offer.VideoCrypto.MasterKey),
			MasterSalt: []byte(offer.VideoCrypto.MasterSalt),
		},
	}
	c.audioSession = &srtp.Session{
		Remote: &srtp.Endpoint{
			Addr:       offer.Address.IPAddr,
			Port:       offer.Address.AudioRTPPort,
			MasterKey:  []byte(offer.AudioCrypto.MasterKey),
			MasterSalt: []byte(offer.AudioCrypto.MasterSalt),
		},
	}
}

func (c *Consumer) GetAnswer() *camera.SetupEndpointsResponse {
	c.videoSession.Local = c.srtpEndpoint()
	c.audioSession.Local = c.srtpEndpoint()

	return &camera.SetupEndpointsResponse{
		SessionID: c.sessionID,
		Status:    camera.StreamingStatusAvailable,
		Address: camera.Address{
			IPAddr:       c.videoSession.Local.Addr,
			VideoRTPPort: c.videoSession.Local.Port,
			AudioRTPPort: c.audioSession.Local.Port,
		},
		VideoCrypto: camera.SRTPCryptoSuite{
			MasterKey:  string(c.videoSession.Local.MasterKey),
			MasterSalt: string(c.videoSession.Local.MasterSalt),
		},
		AudioCrypto: camera.SRTPCryptoSuite{
			MasterKey:  string(c.audioSession.Local.MasterKey),
			MasterSalt: string(c.audioSession.Local.MasterSalt),
		},
		VideoSSRC: c.videoSession.Local.SSRC,
		AudioSSRC: c.audioSession.Local.SSRC,
	}
}

func (c *Consumer) SetConfig(conf *camera.SelectedStreamConfiguration) bool {
	if c.sessionID != conf.Control.SessionID {
		return false
	}

	c.SDP = fmt.Sprintf("%+v\n%+v", conf.VideoCodec, conf.AudioCodec)

	c.videoSession.Remote.SSRC = conf.VideoCodec.RTPParams[0].SSRC
	c.videoSession.PayloadType = conf.VideoCodec.RTPParams[0].PayloadType
	c.videoSession.RTCPInterval = toDuration(conf.VideoCodec.RTPParams[0].RTCPInterval)

	c.audioSession.Remote.SSRC = conf.AudioCodec.RTPParams[0].SSRC
	c.audioSession.PayloadType = conf.AudioCodec.RTPParams[0].PayloadType
	c.audioSession.RTCPInterval = toDuration(conf.AudioCodec.RTPParams[0].RTCPInterval)
	c.audioRTPTime = conf.AudioCodec.CodecParams[0].RTPTime[0]
	c.audioCodec = selectedAudioCodec(&conf.AudioCodec)

	c.srtp.AddSession(c.videoSession)
	c.srtp.AddSession(c.audioSession)

	return true
}

func (c *Consumer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if codec.Kind() != core.KindAudio {
		return nil, core.ErrCantGetTrack
	}

	actualCodec := c.audioCodec
	if actualCodec == nil {
		actualCodec = codec
	}

	c.backTrack = core.NewReceiver(media, actualCodec)
	receiver := c.backTrack

	if !sameAudioCodec(actualCodec, codec) {
		if bridge, err := newAudioTranscoder(c.backTrack, codec); err == nil {
			c.closers = append(c.closers, bridge)
			receiver = bridge.Receiver()
		}
	}

	c.audioSession.OnReadRTP = func(packet *rtp.Packet) {
		c.backTrack.WriteRTP(packet)
		c.Recv += len(packet.Payload)
	}

	c.Receivers = append(c.Receivers, receiver)
	return receiver, nil
}

func (c *Consumer) Start() error {
	return nil
}

func (c *Consumer) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	var session *srtp.Session
	if codec.Kind() == core.KindVideo {
		session = c.videoSession
	} else {
		session = c.audioSession
	}

	sender := core.NewSender(media, track.Codec)

	if c.deadline == nil {
		c.deadline = time.NewTimer(time.Second * 30)

		sender.Handler = func(packet *rtp.Packet) {
			c.deadline.Reset(core.ConnDeadline)
			if n, err := session.WriteRTP(packet); err == nil {
				c.Send += n
			}
		}
	} else {
		sender.Handler = func(packet *rtp.Packet) {
			if n, err := session.WriteRTP(packet); err == nil {
				c.Send += n
			}
		}
	}

	switch codec.Name {
	case core.CodecH264:
		sender.Handler = h264.RTPPay(1378, sender.Handler)
		if track.Codec.IsRTP() {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
		}
	case core.CodecOpus:
		sender.Handler = opus.RepackToHAP(c.audioRTPTime, sender.Handler)
	}

	sender.HandleRTP(track)
	c.Senders = append(c.Senders, sender)
	return nil
}

func (c *Consumer) WriteTo(io.Writer) (int64, error) {
	if c.deadline != nil {
		<-c.deadline.C
	}
	return 0, nil
}

func (c *Consumer) Stop() error {
	if c.deadline != nil {
		c.deadline.Reset(0)
	}
	for _, closer := range c.closers {
		_ = closer.Close()
	}
	return c.Connection.Stop()
}

func (c *Consumer) srtpEndpoint() *srtp.Endpoint {
	addr := c.conn.LocalAddr().(*net.TCPAddr)
	return &srtp.Endpoint{
		Addr:       addr.IP.To4().String(),
		Port:       uint16(c.srtp.Port()),
		MasterKey:  []byte(core.RandString(16, 0)),
		MasterSalt: []byte(core.RandString(14, 0)),
		SSRC:       rand.Uint32(),
	}
}

func toDuration(seconds float32) time.Duration {
	return time.Duration(seconds * float32(time.Second))
}

func selectedAudioCodec(conf *camera.AudioCodecConfiguration) *core.Codec {
	media := audioToMedia([]camera.AudioCodecConfiguration{*conf})
	if len(media.Codecs) == 0 {
		return nil
	}
	if media.Codecs[0].Name == core.CodecOpus {
		codec := media.Codecs[0].Clone()
		codec.ClockRate = 48000
		codec.Channels = 2
		codec.PayloadType = 110
		return codec
	}
	return media.Codecs[0]
}

func sameAudioCodec(src, dst *core.Codec) bool {
	return src.Name == dst.Name &&
		(src.ClockRate == dst.ClockRate || src.ClockRate == 0 || dst.ClockRate == 0) &&
		(src.Channels == dst.Channels || src.Channels == 0 || dst.Channels == 0)
}
