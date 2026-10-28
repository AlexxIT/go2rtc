package webrtc

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Conn struct {
	core.Connection
	core.Listener

	Mode core.Mode `json:"mode"`

	// IgnoreDisconnected - don't close connection on transient `disconnected` state,
	// wait for `failed` or `closed` instead. Pion fires `disconnected` after 5 seconds
	// without incoming packets and can return to `connected` when packets resume,
	// or fires `failed` after 25 more seconds.
	// Useful for producers with expensive reconnect (ex. Nest with API rate limits).
	IgnoreDisconnected bool

	// KeyframeInterval - for active producers with H264 video: send RTCP PLI
	// (keyframe request) when no keyframe was received for this long.
	// Some remote senders (ex. Nest) send the first keyframe late or rarely,
	// so consumers like ffmpeg give up before they can start decoding.
	// Zero disables keyframe requests (default).
	KeyframeInterval time.Duration

	pc *webrtc.PeerConnection

	offer  string
	closed core.Waiter
}

func NewConn(pc *webrtc.PeerConnection) *Conn {
	c := &Conn{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "webrtc",
			Transport:  pc,
		},
		pc: pc,
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		// last candidate will be empty
		if candidate != nil {
			c.Fire(candidate)
		}
	})

	pc.OnDataChannel(func(channel *webrtc.DataChannel) {
		c.Fire(channel)
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		if state != webrtc.ICEConnectionStateChecking {
			return
		}
		pc.SCTP().Transport().ICETransport().OnSelectedCandidatePairChange(
			func(pair *webrtc.ICECandidatePair) {
				// fix situation when candidate pair changes multiple times
				if i := strings.IndexByte(c.Protocol, '+'); i > 0 {
					c.Protocol = c.Protocol[:i]
				}
				c.Protocol += "+" + pair.Remote.Protocol.String()
				c.RemoteAddr = fmt.Sprintf(
					"%s:%d %s", sanitizeIP6(pair.Remote.Address), pair.Remote.Port, pair.Remote.Typ,
				)
				if pair.Remote.RelatedAddress != "" {
					c.RemoteAddr += fmt.Sprintf(
						" %s:%d", sanitizeIP6(pair.Remote.RelatedAddress), pair.Remote.RelatedPort,
					)
				}
			},
		)
	})

	pc.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		media, codec := c.getMediaCodec(remote)
		if media == nil {
			return
		}

		track, err := c.GetTrack(media, codec)
		if err != nil {
			return
		}

		switch c.Mode {
		case core.ModePassiveProducer, core.ModeActiveProducer:
			// replace the theoretical list of codecs with the actual list of codecs
			if len(media.Codecs) > 1 {
				media.Codecs = []*core.Codec{codec}
			}
		}

		if c.Mode == core.ModePassiveProducer && remote.Kind() == webrtc.RTPCodecTypeVideo {
			go func() {
				pkts := []rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remote.SSRC())}}
				for range time.NewTicker(time.Second * 2).C {
					if err := pc.WriteRTCP(pkts); err != nil {
						return
					}
				}
			}()
		}

		// keyframe requests for active producers (ex. Nest), H264 only
		var lastKeyframe atomic.Int64 // unix nanos, 0 = never
		requestKeyframes := c.Mode == core.ModeActiveProducer && c.KeyframeInterval > 0 &&
			remote.Kind() == webrtc.RTPCodecTypeVideo && codec.Name == core.CodecH264
		if requestKeyframes {
			interval := c.KeyframeInterval
			go func() {
				pkts := []rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remote.SSRC())}}
				ticker := time.NewTicker(time.Second * 2)
				defer ticker.Stop()
				for range ticker.C {
					if time.Since(time.Unix(0, lastKeyframe.Load())) < interval {
						continue
					}
					if err := pc.WriteRTCP(pkts); err != nil {
						return
					}
				}
			}()
		}

		for {
			b := make([]byte, ReceiveMTU)
			n, _, err := remote.Read(b)
			if err != nil {
				return
			}

			c.Recv += n

			packet := &rtp.Packet{}
			if err := packet.Unmarshal(b[:n]); err != nil {
				return
			}

			if len(packet.Payload) == 0 {
				continue
			}

			if requestKeyframes && isH264KeyframeRTP(packet.Payload) {
				lastKeyframe.Store(time.Now().UnixNano())
			}

			track.WriteRTP(packet)
		}
	})

	// OK connection:
	// 15:01:46 ICE connection state changed: checking
	// 15:01:46 peer connection state changed: connected
	// 15:01:54 peer connection state changed: disconnected
	// 15:02:20 peer connection state changed: failed
	//
	// Fail connection:
	// 14:53:08 ICE connection state changed: checking
	// 14:53:39 peer connection state changed: failed
	pc.OnConnectionStateChange(c.onConnectionStateChange)

	return c
}

func (c *Conn) onConnectionStateChange(state webrtc.PeerConnectionState) {
	c.Fire(state)

	switch state {
	case webrtc.PeerConnectionStateConnected:
		for _, sender := range c.Senders {
			sender.Start()
		}
	case webrtc.PeerConnectionStateDisconnected:
		// disconnect event comes earlier, than failed
		// but it comes only for success connections
		if c.IgnoreDisconnected {
			return
		}
		_ = c.Close()
	case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
		_ = c.Close()
	}
}

func (c *Conn) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.Connection)
}

func (c *Conn) Close() error {
	c.closed.Done(nil)
	return c.pc.Close()
}

func (c *Conn) AddCandidate(candidate string) error {
	// pion uses only candidate value from json/object candidate struct
	return c.pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: candidate})
}

func (c *Conn) GetSenderTrack(mid string) *Track {
	if tr := c.getTranseiver(mid); tr != nil {
		if s := tr.Sender(); s != nil {
			if t := s.Track().(*Track); t != nil {
				return t
			}
		}
	}
	return nil
}

func (c *Conn) getTranseiver(mid string) *webrtc.RTPTransceiver {
	for _, tr := range c.pc.GetTransceivers() {
		if tr.Mid() == mid {
			return tr
		}
	}
	return nil
}

func (c *Conn) getMediaCodec(remote *webrtc.TrackRemote) (*core.Media, *core.Codec) {
	for _, tr := range c.pc.GetTransceivers() {
		// search Transeiver for this TrackRemote
		if tr.Receiver() == nil || tr.Receiver().Track() != remote {
			continue
		}

		// search Media for this MID
		for _, media := range c.Medias {
			if media.ID != tr.Mid() || media.Direction != core.DirectionRecvonly {
				continue
			}

			// search codec for this PayloadType
			for _, codec := range media.Codecs {
				if codec.PayloadType != uint8(remote.PayloadType()) {
					continue
				}
				return media, codec
			}
		}
	}

	// fix moment when core.ModePassiveProducer or core.ModeActiveProducer
	// sends new codec with new payload type to same media
	// check GetTrack
	panic(core.Caller())

	return nil, nil
}

func sanitizeIP6(host string) string {
	if strings.IndexByte(host, ':') > 0 {
		return "[" + host + "]"
	}
	return host
}

// isH264KeyframeRTP reports whether an H264 RTP payload starts a keyframe:
// a single IDR or SPS NALU, a STAP-A aggregate containing one, or the first
// fragment (FU-A) of an IDR.
func isH264KeyframeRTP(payload []byte) bool {
	if len(payload) < 2 {
		return false
	}
	switch naluType := payload[0] & 0x1F; naluType {
	case 5, 7: // IDR, SPS
		return true
	case 24: // STAP-A
		for b := payload[1:]; len(b) >= 3; {
			size := int(b[0])<<8 | int(b[1])
			if t := b[2] & 0x1F; t == 5 || t == 7 {
				return true
			}
			if 2+size > len(b) {
				return false
			}
			b = b[2+size:]
		}
	case 28: // FU-A, start bit set and original type IDR
		return payload[1]&0x80 != 0 && payload[1]&0x1F == 5
	}
	return false
}
