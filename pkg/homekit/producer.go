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

	// two-way audio: one sender for all consumers, audio session and its clock rate while started
	backchannel   *core.Sender
	backchannelMu sync.Mutex
	txSession     atomic.Pointer[srtp.Session]
	txClockRate   atomic.Uint32

	Backchannel bool `json:"-"` // offer two-way audio, if the camera supports Opus

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

	char := acc.GetCharacter(camera.TypeSupportedVideoStreamConfiguration)
	if char == nil {
		return nil
	}
	if err = char.ReadTLV8(&c.videoConfig); err != nil {
		return nil
	}

	char = acc.GetCharacter(camera.TypeSupportedAudioStreamConfiguration)
	if char == nil {
		return nil
	}
	if err = char.ReadTLV8(&c.audioConfig); err != nil {
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

	if c.Backchannel {
		if media := backchannelMedia(c.audioConfig.Codecs); media != nil {
			c.Medias = append(c.Medias, media)
		}
	}

	return c.Medias
}

// backchannelMedia - two-way audio media for cameras with an Opus audio stream
func backchannelMedia(codecs []camera.AudioCodecConfiguration) *core.Media {
	if opusConfig(codecs) == nil {
		return nil
	}
	return &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs: []*core.Codec{
			{Name: core.CodecOpus, ClockRate: 48000, Channels: 2},
		},
	}
}

func opusConfig(codecs []camera.AudioCodecConfiguration) *camera.AudioCodecConfiguration {
	for i := range codecs {
		if codecs[i].CodecType == camera.AudioCodecTypeOpus {
			return &codecs[i]
		}
	}
	return nil
}

func (c *Client) Start() error {
	c.backchannelMu.Lock()
	backchannel := c.backchannel != nil
	c.backchannelMu.Unlock()

	if c.Receivers == nil && !backchannel {
		return errors.New("producer without tracks")
	}

	if c.Receivers != nil && c.Receivers[0].Codec.Name == core.CodecJPEG {
		return c.startMJPEG()
	}

	videoTrack := c.trackByKind(core.KindVideo)
	videoCodec := trackToVideo(videoTrack, &c.videoConfig.Codecs[0], c.MaxWidth, c.MaxHeight)

	audioTrack := c.trackByKind(core.KindAudio)
	audio0 := &c.audioConfig.Codecs[0]
	if audioTrack == nil && backchannel {
		audio0 = opusConfig(c.audioConfig.Codecs) // two-way audio only: request Opus from the camera
	}
	audioCodec := trackToAudio(audioTrack, audio0)

	c.videoSession = &srtp.Session{Local: c.srtpEndpoint()}
	c.audioSession = &srtp.Session{Local: c.srtpEndpoint()}

	var err error
	c.stream, err = camera.NewStream(c.hap, videoCodec, audioCodec, c.videoSession, c.audioSession, c.Bitrate)
	if err != nil {
		return err
	}

	// RTP params selected by NewStream, for sending audio to the camera, before AddSession (RTCP timer)
	if params := audioCodec.RTPParams; len(params) > 0 {
		c.audioSession.PayloadType = params[0].PayloadType
		c.audioSession.RTCPInterval = toDuration(params[0].RTCPInterval)
	}

	c.srtp.AddSession(c.videoSession)
	c.srtp.AddSession(c.audioSession)

	c.startBackchannel(audioCodec)

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
		// two-way audio only: the camera video keeps the stream alive
		c.videoSession.OnReadRTP = func(packet *rtp.Packet) {
			deadline.Reset(core.ConnDeadline)
		}
	}

	if c.audioSession.OnReadRTP != nil {
		c.audioSession.OnReadRTP = timekeeper(c.audioSession.OnReadRTP)
	}

	<-deadline.C

	return nil
}

// AddTrack - backchannel: sends the consumer audio to the camera speaker
// over the audio session of the started stream.
func (c *Client) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	if track.Codec.Name != core.CodecOpus {
		return errors.New("homekit: unsupported backchannel codec: " + track.Codec.Name)
	}

	c.backchannelMu.Lock()
	defer c.backchannelMu.Unlock()

	if c.backchannel == nil {
		c.backchannel = core.NewSender(media, track.Codec)
		c.backchannel.Handler = c.backchannelHandler(time.Now)
		c.Senders = append(c.Senders, c.backchannel)
	}

	c.backchannel.HandleRTP(track)
	return nil
}

// startBackchannel enables sending, if the audio session uses Opus and was added to the SRTP server
func (c *Client) startBackchannel(audioCodec *camera.AudioCodecConfiguration) {
	if audioCodec.CodecType != camera.AudioCodecTypeOpus {
		return
	}

	i := int(audioCodec.CodecParams[0].SampleRate[0])
	if i >= len(audioSampleRates) {
		return
	}

	if c.srtp.GetSession(c.audioSession.Remote.SSRC) != c.audioSession {
		return // AddSession failed
	}

	c.txClockRate.Store(audioSampleRates[i])
	c.txSession.Store(c.audioSession)
}

// gap in the consumer audio after which a new talk spurt starts
const backchannelSpurtGap = 500 * time.Millisecond

// backchannelHandler rewrites the consumer packets for the camera: continuous RTP sequence,
// timestamps rescaled to the camera clock rate and continuous across talk spurts,
// marker on the first packet of a spurt.
func (c *Client) backchannelHandler(now func() time.Time) core.HandlerFunc {
	const inClockRate = 48000 // RTP clock rate for Opus is always 48000 (RFC 7587)

	seq := uint16(rand.Uint32())

	var started bool
	var inBase, inLast, outBase, outLast uint32
	var lastTime time.Time

	return func(packet *rtp.Packet) {
		session := c.txSession.Load()
		if session == nil || len(packet.Payload) == 0 {
			return // stream not started, already stopped or empty packet
		}

		rate := uint64(c.txClockRate.Load())
		t := now()

		// new spurt: first packet, silence or jump (backward or more than 1 s) in consumer timestamps
		spurt := !started || t.Sub(lastTime) > backchannelSpurtGap || packet.Timestamp-inLast > inClockRate
		if spurt {
			if started {
				// continue the camera clock with the real time since the last packet
				elapsed := uint64(t.Sub(lastTime) / time.Millisecond)
				outBase = outLast + uint32(elapsed*rate/1000)
			}
			inBase = packet.Timestamp
			started = true
		}

		out := outBase + uint32(uint64(packet.Timestamp-inBase)*rate/inClockRate)

		// rebase long spurts before the consumer timestamps difference can wrap
		if packet.Timestamp-inBase > 1<<30 {
			inBase, outBase = packet.Timestamp, out
		}

		clone := *packet
		clone.Marker = spurt
		clone.SequenceNumber = seq
		clone.Timestamp = out

		seq++
		inLast, outLast, lastTime = packet.Timestamp, out, t

		if n, err := session.WriteRTP(&clone); err == nil {
			c.Send += n
		}
	}
}

func (c *Client) Stop() error {
	c.txSession.Store(nil)

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
