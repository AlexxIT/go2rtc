package webrtc

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Conn struct {
	core.Connection
	core.Listener

	Mode core.Mode `json:"mode"`

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

		// Request periodic keyframes from passive producers (WHIP, browser push)
		// and from the Nest source. Nest is an active-pull producer whose keyframe
		// interval drifts long when idle, so a consumer joining mid-GOP would wait
		// up to a whole interval to start. Gate on the format name rather than the
		// mode: other active-pull WebRTC sources (battery cameras) should not be
		// forced into a 2 s IDR cadence.
		if (c.Mode == core.ModePassiveProducer || c.FormatName == "nest/webrtc") && remote.Kind() == webrtc.RTPCodecTypeVideo {
			go func() {
				mediaSSRC := uint32(remote.SSRC())
				pkts := []rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: mediaSSRC}}
				// Google reportedly ignores bare PLI but honors FIR (#2365); both are
				// negotiated in the offer. FIR needs an incrementing sequence number
				// (RFC 5104) or repeated requests are deduplicated.
				var fir *rtcp.FullIntraRequest
				if c.FormatName == "nest/webrtc" {
					fir = &rtcp.FullIntraRequest{
						MediaSSRC: mediaSSRC,
						FIR:       []rtcp.FIREntry{{SSRC: mediaSSRC}},
					}
					pkts = append(pkts, fir)
				}
				t := time.NewTicker(time.Second * 2)
				defer t.Stop()
				for range t.C {
					if fir != nil {
						fir.FIR[0].SequenceNumber++
					}
					if err := pc.WriteRTCP(pkts); err != nil {
						return
					}
				}
			}()
		}

		// Google's SDP answer carries profile-level-id but no sprop-parameter-sets;
		// SPS/PPS arrive in-band, usually bundled in a STAP-A. Capture them once and
		// append them to the codec's fmtp line so RTSP consumers learn the video
		// dimensions from the DESCRIBE SDP instead of waiting for an in-band
		// keyframe and a probe. pkg/dvrip does the equivalent for H265.
		captureSprop := c.FormatName == "nest/webrtc" && codec.Name == core.CodecH264 &&
			!strings.Contains(codec.FmtpLine, "sprop-parameter-sets=")
		var spropSPS, spropPPS []byte

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

			if captureSprop {
				save := func(nal []byte) {
					if len(nal) == 0 {
						return
					}
					switch nal[0] & 0x1F {
					case h264.NALUTypeSPS:
						spropSPS = append([]byte(nil), nal...)
					case h264.NALUTypePPS:
						spropPPS = append([]byte(nil), nal...)
					}
				}
				if pl := packet.Payload; pl[0]&0x1F == 24 { // STAP-A: bundled NALs
					for bb := pl[1:]; len(bb) >= 2; {
						sz := int(binary.BigEndian.Uint16(bb))
						bb = bb[2:]
						if sz < 1 || sz > len(bb) {
							break
						}
						save(bb[:sz])
						bb = bb[sz:]
					}
				} else {
					save(pl)
				}
				if spropSPS != nil && spropPPS != nil {
					if codec.FmtpLine != "" {
						codec.FmtpLine += ";"
					}
					codec.FmtpLine += "sprop-parameter-sets=" +
						base64.StdEncoding.EncodeToString(spropSPS) + "," +
						base64.StdEncoding.EncodeToString(spropPPS)
					captureSprop = false
				}
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
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.Fire(state)

		switch state {
		case webrtc.PeerConnectionStateConnected:
			for _, sender := range c.Senders {
				sender.Start()
			}
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			// disconnect event comes earlier, than failed
			// but it comes only for success connections
			_ = c.Close()
		}
	})

	return c
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
