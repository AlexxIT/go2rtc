package homekit

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/pion/rtp"
)

// Deprecated: rename to Producer
type Client struct {
	core.Connection

	hap  *hap.Client
	srtp *srtp.Server

	videoConfig camera.SupportedVideoStreamConfiguration
	audioConfig camera.SupportedAudioStreamConfiguration

	videoSession *srtp.Session
	audioSession *srtp.Session

	stream *camera.Stream

	// The backchannel is sent on the audio session once the stream has
	// started with the backchannel's codec. Tracks can be added before or
	// after the start (e.g. a microphone added during live view).
	mu                 sync.Mutex
	backchannelCodec   string
	startedAudio       *srtp.Session
	startedCodec       string
	backchannelSession atomic.Pointer[srtp.Session]

	MaxWidth  int `json:"-"`
	MaxHeight int `json:"-"`
	Bitrate   int `json:"-"` // in bits/s
}

func Dial(rawURL string, server *srtp.Server) (*Client, error) {
	conn, err := hap.Dial(rawURL)
	if err != nil {
		return nil, err
	}

	client := &Client{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "homekit",
			Protocol:   "udp",
			RemoteAddr: conn.Conn.RemoteAddr().String(),
			Source:     rawURL,
			Transport:  conn,
		},
		hap:  conn,
		srtp: server,
	}

	return client, nil
}

func (c *Client) Conn() net.Conn {
	return c.hap.Conn
}

func (c *Client) GetMedias() []*core.Media {
	if c.Medias != nil {
		return c.Medias
	}

	acc, err := c.hap.GetFirstAccessory()
	if err != nil {
		return nil
	}

	// Some cameras (ex. Logitech Circle 2) expose several RTP stream
	// services and the first one is a stub: streaming status "unavailable"
	// and a video config carrying no VideoAttrs. acc.GetCharacter() scans
	// the whole accessory and returns that one. Pick the richest service
	// that actually parses, like GetFreeStream() does on the stream side.
	found := false
	for _, srv := range acc.Services {
		charVideo := srv.GetCharacter(camera.TypeSupportedVideoStreamConfiguration)
		charAudio := srv.GetCharacter(camera.TypeSupportedAudioStreamConfiguration)
		if charVideo == nil || charAudio == nil {
			continue
		}

		var videoConfig camera.SupportedVideoStreamConfiguration
		if charVideo.ReadTLV8(&videoConfig) != nil || len(videoConfig.Codecs) == 0 ||
			len(videoConfig.Codecs[0].CodecParams) == 0 {
			continue
		}

		var audioConfig camera.SupportedAudioStreamConfiguration
		if charAudio.ReadTLV8(&audioConfig) != nil || len(audioConfig.Codecs) == 0 ||
			len(audioConfig.Codecs[0].CodecParams) == 0 {
			continue
		}

		if !found || len(videoConfig.Codecs[0].VideoAttrs) > len(c.videoConfig.Codecs[0].VideoAttrs) {
			c.videoConfig = videoConfig
			c.audioConfig = audioConfig
			found = true
		}
	}
	if !found {
		return nil
	}

	c.SDP = fmt.Sprintf("%+v\n%+v", c.videoConfig, c.audioConfig)

	c.Medias = []*core.Media{
		videoToMedia(c.videoConfig.Codecs),
		audioToMedia(c.audioConfig.Codecs),
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs: []*core.Codec{
				{
					Name:        core.CodecJPEG,
					ClockRate:   90000,
					PayloadType: core.PayloadTypeRAW,
				},
			},
		},
	}

	if media := backchannelMedia(acc, c.audioConfig.Codecs); media != nil {
		c.Medias = append(c.Medias, media)
	}

	return c.Medias
}

func (c *Client) Start() error {
	if c.Receivers == nil && c.Senders == nil {
		return errors.New("producer without tracks")
	}

	if c.Receivers != nil && c.Receivers[0].Codec.Name == core.CodecJPEG {
		return c.startMJPEG()
	}

	videoTrack := c.trackByKind(core.KindVideo)
	videoCodec := trackToVideo(videoTrack, &c.videoConfig.Codecs[0], c.MaxWidth, c.MaxHeight)

	audioTrack := c.trackByKind(core.KindAudio)
	var audioCodec *camera.AudioCodecConfiguration
	c.mu.Lock()
	backchannelCodec := c.backchannelCodec
	c.mu.Unlock()
	if audioTrack == nil && backchannelCodec != "" {
		// only the backchannel needs audio, so it decides the codec
		audioCodec = backchannelToAudio(backchannelCodec, c.audioConfig.Codecs)
	}
	if audioCodec == nil {
		audioCodec = trackToAudio(audioTrack, &c.audioConfig.Codecs[0])
	}

	// Payload types and RTCP intervals as requested in camera.NewStream. They
	// are used for the packets and sender reports this side sends.
	c.videoSession = &srtp.Session{Local: c.srtpEndpoint(), PayloadType: 99, RTCPInterval: 500 * time.Millisecond}
	c.audioSession = &srtp.Session{Local: c.srtpEndpoint(), PayloadType: 110, RTCPInterval: 5 * time.Second}

	var err error
	c.stream, err = camera.NewStream(c.hap, videoCodec, audioCodec, c.videoSession, c.audioSession, c.Bitrate)
	if err != nil {
		return err
	}

	c.srtp.AddSession(c.videoSession)
	c.srtp.AddSession(c.audioSession)

	c.mu.Lock()
	c.startedAudio = c.audioSession
	c.startedCodec = audioCodecs[audioCodec.CodecType]
	c.updateBackchannel()
	c.mu.Unlock()

	deadline := time.NewTimer(core.ConnDeadline)

	if videoTrack != nil {
		c.videoSession.OnReadRTP = func(packet *rtp.Packet) {
			deadline.Reset(core.ConnDeadline)
			videoTrack.WriteRTP(packet)
			c.Recv += len(packet.Payload)
		}

		if audioTrack != nil {
			c.audioSession.OnReadRTP = func(packet *rtp.Packet) {
				audioTrack.WriteRTP(packet)
				c.Recv += len(packet.Payload)
			}
		}
	} else if audioTrack != nil {
		c.audioSession.OnReadRTP = func(packet *rtp.Packet) {
			deadline.Reset(core.ConnDeadline)
			audioTrack.WriteRTP(packet)
			c.Recv += len(packet.Payload)
		}
	} else {
		// backchannel only: the stream lives while the camera sends audio
		c.audioSession.OnReadRTP = func(packet *rtp.Packet) {
			deadline.Reset(core.ConnDeadline)
			c.Recv += len(packet.Payload)
		}
	}

	if audioTrack != nil {
		c.audioSession.OnReadRTP = timekeeper(c.audioSession.OnReadRTP)
	}

	<-deadline.C

	return nil
}

// updateBackchannel sends the backchannel on the started audio session if
// the codecs match: the camera takes audio only in the codec selected for
// the stream. Call with c.mu held.
func (c *Client) updateBackchannel() {
	if c.startedAudio != nil && c.backchannelCodec != "" && c.backchannelCodec == c.startedCodec {
		c.backchannelSession.Store(c.startedAudio)
	}
}

func (c *Client) Stop() error {
	c.mu.Lock()
	c.startedAudio = nil
	c.backchannelSession.Store(nil)
	c.mu.Unlock()
	if c.videoSession != nil && c.videoSession.Remote != nil {
		c.srtp.DelSession(c.videoSession)
	}
	if c.audioSession != nil && c.audioSession.Remote != nil {
		c.srtp.DelSession(c.audioSession)
	}

	return c.Connection.Stop()
}

func (c *Client) trackByKind(kind string) *core.Receiver {
	for _, receiver := range c.Receivers {
		if receiver.Codec.Kind() == kind {
			return receiver
		}
	}
	return nil
}

func (c *Client) startMJPEG() error {
	receiver := c.Receivers[0]

	for {
		b, err := c.hap.GetImage(1920, 1080)
		if err != nil {
			return err
		}

		c.Recv += len(b)

		packet := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: b,
		}
		receiver.WriteRTP(packet)
	}
}

func (c *Client) srtpEndpoint() *srtp.Endpoint {
	return &srtp.Endpoint{
		Addr:       c.hap.LocalIP(),
		Port:       uint16(c.srtp.Port()),
		MasterKey:  []byte(core.RandString(16, 0)),
		MasterSalt: []byte(core.RandString(14, 0)),
		SSRC:       rand.Uint32(),
	}
}

func timekeeper(handler core.HandlerFunc) core.HandlerFunc {
	const sampleRate = 16000
	const sampleSize = 480

	var send time.Duration
	var firstTime time.Time

	return func(packet *rtp.Packet) {
		now := time.Now()

		if send != 0 {
			elapsed := now.Sub(firstTime) * sampleRate / time.Second
			if send+sampleSize > elapsed {
				return // drop overflow frame
			}
		} else {
			firstTime = now
		}

		send += sampleSize

		packet.Timestamp = uint32(send)

		handler(packet)
	}
}
