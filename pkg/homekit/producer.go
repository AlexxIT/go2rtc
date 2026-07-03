package homekit

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
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

	videoSRTP *srtp.Server
	audioSRTP *srtp.Server

	videoConfig camera.SupportedVideoStreamConfiguration
	audioConfig camera.SupportedAudioStreamConfiguration

	videoSession *srtp.Session
	audioSession *srtp.Session

	// audioSend/audioSeq assign strictly increasing RTP timestamp/sequence
	// for the backchannel audio SSRC across separate AddTrack calls (one per
	// talkback playback) and across depacketized AUs within a call. Must not
	// reset per-call: RTP requires monotonic sequence/timestamp for a given
	// SSRC or receivers may treat the stream as stale and drop it.
	audioSend uint32
	audioSeq  uint16

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

	audioMedia := audioToMedia(c.audioConfig.Codecs)
	backchannelCodecs := make([]*core.Codec, 0, len(audioMedia.Codecs)+1)
	backchannelCodecs = append(backchannelCodecs, audioMedia.Codecs...)
	backchannelCodecs = append(backchannelCodecs, &core.Codec{
		Name:      core.CodecAAC,
		ClockRate: 16000,
		Channels:  1,
	})

	c.Medias = append(c.Medias, &core.Media{
		Kind:      core.KindAudio,
		Direction: core.DirectionSendonly,
		Codecs:    backchannelCodecs,
	})

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

	c.videoSRTP = srtp.NewServer(":0")
	if err := c.videoSRTP.Start(); err != nil {
		return err
	}
	c.audioSRTP = srtp.NewServer(":0")
	if err := c.audioSRTP.Start(); err != nil {
		c.videoSRTP.Close()
		return err
	}

	c.videoSession = &srtp.Session{Local: c.srtpEndpoint(c.videoSRTP)}
	c.audioSession = &srtp.Session{Local: c.srtpEndpoint(c.audioSRTP)}

	var err error
	c.stream, err = camera.NewStream(c.hap, videoCodec, audioCodec, c.videoSession, c.audioSession, c.Bitrate)
	if err != nil {
		c.videoSRTP.Close()
		c.audioSRTP.Close()
		return err
	}

	c.videoSession.PayloadType = 99
	c.videoSession.RTCPInterval = 500 * time.Millisecond
	c.audioSession.PayloadType = 110
	c.audioSession.RTCPInterval = 5 * time.Second

	c.videoSRTP.AddSession(c.videoSession)
	c.audioSRTP.AddSession(c.audioSession)

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
		c.audioSession.OnReadRTP = timekeeper(c.audioSession.OnReadRTP)
	}

	<-deadline.C

	return nil
}

func (c *Client) Stop() error {
	if c.videoSession != nil && c.videoSession.Remote != nil {
		c.videoSRTP.DelSession(c.videoSession)
	}
	if c.audioSession != nil && c.audioSession.Remote != nil {
		c.audioSRTP.DelSession(c.audioSession)
	}
	if c.videoSRTP != nil {
		c.videoSRTP.Close()
	}
	if c.audioSRTP != nil {
		c.audioSRTP.Close()
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

func (c *Client) srtpEndpoint(server *srtp.Server) *srtp.Endpoint {
	return &srtp.Endpoint{
		Addr:       c.hap.LocalIP(),
		Port:       uint16(server.Port()),
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

// AddTrack sends backchannel (talkback) audio to the camera.
func (c *Client) AddTrack(media *core.Media, codec *core.Codec, track *core.Receiver) error {
	const sampleRate = 16000
	const sampleSize = 480 // 30ms @ 16kHz, matches negotiated RTPTime
	const frameInterval = sampleSize * time.Second / sampleRate

	switch codec.Name {
	case core.CodecELD, core.CodecOpus, core.CodecAAC:
		sender := core.NewSender(media, track.Codec)

		var lastSent time.Time

		// writeRTP sends exactly one AU per RTP packet, wrapped in the
		// RFC 3640 §3.3.6 "AAC-hbr" AU-header format required by the HAP
		// spec for AAC-ELD (11.9 Media Transport: "RFC 3640 - Section 3.3.6
		// - High Bit-rate AAC for AAC-ELD"; sizeLength=13/indexLength=3, see
		// aac.FMTP, is exactly a 2-byte AU-header): a 2-byte AU-headers-length
		// field followed by one 2-byte AU-header giving the AU size.
		//
		// Timestamp/sequence for the shared backchannel SSRC must persist
		// across calls (RTP requires monotonic sequence/timestamp for a
		// given SSRC), and Marker is always true since every packet is
		// exactly one complete AU.
		//
		// Paced with a rolling minimum spacing (time since the last packet
		// sent, not a fixed schedule from burst start) so consecutive sends
		// are never closer than one frame interval even if upstream delivery
		// (ffmpeg -> internal RTSP loopback -> aac.RTPDepay(), which can
		// unpack several bundled AUs from one incoming packet at once) is
		// bursty.
		writeRTP := func(packet *rtp.Packet) {
			if c.audioSession == nil || c.audioSession.Remote == nil {
				return
			}

			now := time.Now()
			if !lastSent.IsZero() {
				if expected := lastSent.Add(frameInterval); now.Before(expected) {
					time.Sleep(expected.Sub(now))
				}
			}
			lastSent = time.Now()

			auSize := uint16(len(packet.Payload))
			wrapped := make([]byte, 4+auSize)
			wrapped[1] = 16 // AU-headers-length in bits: one 16-bit AU-header
			binary.BigEndian.PutUint16(wrapped[2:], auSize<<3)
			copy(wrapped[4:], packet.Payload)
			packet.Payload = wrapped

			packet.Marker = true
			packet.Timestamp = c.audioSend
			packet.SequenceNumber = c.audioSeq
			c.audioSend += sampleSize
			c.audioSeq++

			if n, err := c.audioSession.WriteRTP(packet); err == nil {
				c.Send += n
			}
		}

		if track.Codec.IsRTP() {
			// ffmpeg's own RTP muxer may bundle multiple AAC frames per
			// packet; split back into one AU per packet before forwarding.
			sender.Handler = aac.RTPDepay(writeRTP)
		} else {
			sender.Handler = writeRTP
		}

		sender.HandleRTP(track)
		c.Senders = append(c.Senders, sender)
	}

	return nil
}
