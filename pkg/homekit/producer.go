package homekit

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
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
		// The accessory only sends SPS/PPS in-band alongside a keyframe, and
		// keyframes are seconds apart (5s measured on an Aqara G410). HAP has no
		// GOP control to shorten that. A consumer attaching between keyframes
		// therefore receives slices referencing a PPS it never saw - ffmpeg
		// reports "non-existing PPS 0 referenced" and gives up before the next
		// keyframe, caching the stream as audio-only. Remember the parameter
		// sets the first time they appear and advertise them out-of-band, so
		// later consumers can decode from the moment they attach.
		var sps, pps []byte

		c.videoSession.OnReadRTP = func(packet *rtp.Packet) {
			deadline.Reset(core.ConnDeadline)

			if sps == nil || pps == nil {
				collectParameterSets(packet.Payload, &sps, &pps)
				if sps != nil && pps != nil {
					videoTrack.Codec.FmtpLine = withParameterSets(videoTrack.Codec.FmtpLine, sps, pps)
				}
			}

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
		c.audioSession.OnReadRTP = timekeeper(c.audioSession.OnReadRTP)
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

// H264 NAL unit types needed to find parameter sets in an RTP payload.
const (
	naluSPS   = 7
	naluPPS   = 8
	naluSTAPA = 24
)

// collectParameterSets extracts H264 SPS/PPS from an RTP payload. Parameter sets
// are small and arrive either as a single NAL or inside a STAP-A aggregate, so
// fragmented (FU) payloads are not considered.
func collectParameterSets(payload []byte, sps, pps *[]byte) {
	if len(payload) == 0 {
		return
	}

	if payload[0]&0x1F != naluSTAPA {
		storeParameterSet(payload, sps, pps)
		return
	}

	b := payload[1:]
	for len(b) >= 2 {
		size := int(binary.BigEndian.Uint16(b))
		b = b[2:]
		if size == 0 || size > len(b) {
			return
		}
		storeParameterSet(b[:size], sps, pps)
		b = b[size:]
	}
}

func storeParameterSet(nalu []byte, sps, pps *[]byte) {
	if len(nalu) == 0 {
		return
	}

	switch nalu[0] & 0x1F {
	case naluSPS:
		if *sps == nil {
			*sps = append([]byte(nil), nalu...)
		}
	case naluPPS:
		if *pps == nil {
			*pps = append([]byte(nil), nalu...)
		}
	}
}

// withParameterSets appends sprop-parameter-sets to an fmtp line. It has to go
// last because h264.GetParameterSet reads up to the next ";" or end of string.
func withParameterSets(fmtp string, sps, pps []byte) string {
	if strings.Contains(fmtp, "sprop-parameter-sets=") {
		return fmtp
	}

	ps := "sprop-parameter-sets=" +
		base64.StdEncoding.EncodeToString(sps) + "," +
		base64.StdEncoding.EncodeToString(pps)

	if fmtp == "" {
		return ps
	}
	return fmtp + ";" + ps
}
