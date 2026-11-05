package homekit

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/opus"
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

	return c.Medias
}

func (c *Client) Start() error {
	if c.Receivers == nil {
		return errors.New("producer without tracks")
	}

	if c.Receivers[0].Codec.Name == core.CodecJPEG {
		return c.startMJPEG()
	}

	videoTrack := c.trackByKind(core.KindVideo)
	videoCodec := trackToVideo(videoTrack, &c.videoConfig.Codecs[0], c.MaxWidth, c.MaxHeight)

	audioTrack := c.trackByKind(core.KindAudio)
	audioCodec := trackToAudio(audioTrack, &c.audioConfig.Codecs[0])

	c.videoSession = &srtp.Session{Local: c.srtpEndpoint()}
	c.audioSession = &srtp.Session{Local: c.srtpEndpoint()}

	var err error
	c.stream, err = camera.NewStream(c.hap, videoCodec, audioCodec, c.videoSession, c.audioSession, c.Bitrate)
	if err != nil {
		return err
	}

	c.srtp.AddSession(c.videoSession)
	c.srtp.AddSession(c.audioSession)

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
	} else {
		c.audioSession.OnReadRTP = func(packet *rtp.Packet) {
			deadline.Reset(core.ConnDeadline)
			audioTrack.WriteRTP(packet)
			c.Recv += len(packet.Payload)
		}
	}

	if c.audioSession.OnReadRTP != nil {
		if audioTrack != nil && audioTrack.Codec.Name == core.CodecOpus && audioTrack.Codec.ClockRate != 0 {
			c.audioSession.OnReadRTP = timekeeperOpus(c.audioSession.OnReadRTP, audioTrack.Codec.ClockRate, time.Now)
		} else {
			c.audioSession.OnReadRTP = timekeeper(c.audioSession.OnReadRTP)
		}
	}

	<-deadline.C

	return nil
}

func (c *Client) Stop() error {
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

const (
	// accepted burst of the camera audio before dropping packets
	timekeeperMaxAhead = 250 * time.Millisecond
	// gap in the camera audio after which timestamps jump to real time
	timekeeperMaxBehind = time.Second
	// longer sequence gaps are not counted as lost packets (reordering, restart)
	timekeeperMaxLost = 50
)

// timekeeperOpus rewrites the timestamps of the camera Opus audio and drops overflow packets.
// Unlike timekeeper (fixed 480 samples @ 16 kHz per packet), the duration of each packet
// comes from its TOC (RFC 6716), lost packets are counted and timestamps use the track clock rate.
func timekeeperOpus(handler core.HandlerFunc, clockRate uint32, now func() time.Time) core.HandlerFunc {
	rate := uint64(clockRate)

	var start time.Time
	var send time.Duration // duration of the audio already sent
	var ts uint32
	var seq uint16

	return func(packet *rtp.Packet) {
		if len(packet.Payload) == 0 {
			return
		}

		dur := opusDuration(packet.Payload)
		if dur == 0 {
			dur = audioRTPTime * time.Millisecond // invalid TOC: requested packet time
		}

		t := now()

		lost := packet.SequenceNumber - seq - 1
		seq = packet.SequenceNumber

		if start.IsZero() {
			start = t
		} else {
			// lost packets: their time passes, as if they had been sent
			if lost > 0 && lost <= timekeeperMaxLost {
				send += time.Duration(lost) * dur
				ts += uint32(uint64(lost) * uint64(dur) * rate / uint64(time.Second))
			}

			elapsed := t.Sub(start)
			if send > elapsed+timekeeperMaxAhead {
				return // drop overflow packet
			}
			if gap := elapsed - send; gap > timekeeperMaxBehind {
				send += gap
				ts += uint32(uint64(gap/time.Millisecond) * rate / 1000)
			}
		}

		packet.Timestamp = ts

		send += dur
		ts += uint32(uint64(dur) * rate / uint64(time.Second))

		handler(packet)
	}
}

// opusDuration returns the audio duration of an Opus packet from its TOC (RFC 6716, 3.1),
// or zero for an invalid packet
func opusDuration(payload []byte) time.Duration {
	if len(payload) == 0 {
		return 0
	}

	header := opus.UnmarshalHeader(payload)

	frames := time.Duration(header.Frames)
	if header.Frames == 0xFF {
		// code 3: the number of frames is in the second byte (1..48)
		if len(payload) < 2 {
			return 0
		}
		if frames = time.Duration(payload[1] & 0b111111); frames == 0 || frames > 48 {
			return 0
		}
	}

	if dur := header.FrameSize * frames; dur <= 120*time.Millisecond {
		return dur
	}
	return 0
}
