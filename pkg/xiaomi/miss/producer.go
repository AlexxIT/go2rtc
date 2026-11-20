package miss

import (
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	client  *Client
	initial []*Packet
}

func Dial(rawURL string) (core.Producer, error) {
	client, err := NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	err = client.StartMedia(query.Get("channel"), query.Get("subtype"), query.Get("audio"))
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	medias, initial, err := probe(client, query.Get("audio") != "0")
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/miss",
			Protocol:   client.Protocol(),
			RemoteAddr: client.RemoteAddr().String(),
			UserAgent:  client.Version(),
			Medias:     medias,
			Transport:  client,
		},
		client:  client,
		initial: initial,
	}, nil
}

func probe(client *Client, audio bool) ([]*core.Media, []*Packet, error) {
	firstPacket := true
	firstVideo := true
	seenVideo := false
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))

	var vcodec, acodec *core.Codec
	var sets parameterSets
	var initial []*Packet
	var initialBytes int

	for {
		pkt, err := client.ReadPacket()
		if err != nil {
			if vcodec == nil {
				err = fmt.Errorf("no video")
			}
			return nil, nil, fmt.Errorf("xiaomi: probe: %w", err)
		}
		if firstPacket {
			firstPacket = false
		}

		switch pkt.CodecID {
		case codecH264:
			buf := annexb.EncodeToAVCC(pkt.Payload)
			if firstVideo {
				firstVideo = false
			}
			sets.absorb(buf, false)
			seenVideo = true
			if vcodec == nil && sets.ready() {
				vcodec = h264.AVCCToCodec(sets.avcc)
			}
		case codecH265:
			buf := annexb.EncodeToAVCC(pkt.Payload)
			if firstVideo {
				firstVideo = false
			}
			sets.absorb(buf, true)
			seenVideo = true
			if vcodec == nil && sets.ready() {
				vcodec = h265.AVCCToCodec(sets.avcc)
			}
		case codecPCMA:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: pkt.SampleRate()}
			}
		case codecOPUS:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
			}
		}

		if seenVideo {
			initialBytes += len(pkt.Payload)
			if initialBytes > 4*1024*1024 {
				return nil, nil, fmt.Errorf("xiaomi: probe: initial media exceeds limit")
			}
			initial = append(initial, pkt)
		}
		// Parameter sets are enough to open the picture. The door card is muted,
		// so an absent audio packet must not hold this header or the following keyframe.
		if vcodec != nil {
			break
		}
	}

	_ = client.SetDeadline(time.Time{})

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
	}

	if acodec != nil {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		})

		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{acodec.Clone()},
		})
	}

	return medias, initial, nil
}

// parameterSets collects HEVC VPS+SPS+PPS, or H264 SPS+PPS, across access units.
// HEVC is ready at VPS+SPS. A PPS already in those units is kept; a later one is not a reason to wait.
type parameterSets struct {
	hevc bool
	vps  bool
	sps  bool
	pps  bool
	avcc []byte
}

func (p *parameterSets) absorb(buf []byte, hevc bool) {
	p.hevc = hevc
	for len(buf) >= 5 {
		size := int(binary.BigEndian.Uint32(buf))
		if size < 1 || size > len(buf)-4 {
			return
		}
		typ := buf[4] & 0x1F
		if hevc {
			typ = buf[4] >> 1 & 0x3F
		}
		keep := false
		switch {
		case hevc && typ == h265.NALUTypeVPS:
			p.vps = true
			keep = true
		case hevc && typ == h265.NALUTypeSPS:
			p.sps = true
			keep = true
		case hevc && typ == h265.NALUTypePPS:
			p.pps = true
			keep = true
		case !hevc && typ == h264.NALUTypeSPS:
			p.sps = true
			keep = true
		case !hevc && typ == h264.NALUTypePPS:
			p.pps = true
			keep = true
		}
		if keep {
			p.avcc = append(p.avcc, buf[:4+size]...)
		}
		buf = buf[4+size:]
	}
}

func (p *parameterSets) ready() bool {
	if p.hevc {
		return p.vps && p.sps
	}
	return p.sps && p.pps
}

func (p *parameterSets) label() string {
	var parts []string
	if p.vps {
		parts = append(parts, "vps")
	}
	if p.sps {
		parts = append(parts, "sps")
	}
	if p.pps {
		parts = append(parts, "pps")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}

const timestamp40ms = 48000 * 0.040

// Access units may begin with AUD/SEI before the codec parameter sets.
func containsNALU(data []byte, want byte, hevc bool) bool {
	for len(data) >= 5 {
		size := int(binary.BigEndian.Uint32(data))
		if size < 1 || size > len(data)-4 {
			return false
		}
		typ := data[4] & 0x1F
		if hevc {
			typ = data[4] >> 1 & 0x3F
		}
		if typ == want {
			return true
		}
		data = data[4+size:]
	}
	return false
}

func (p *Producer) Start() error {
	var audioTS uint32

	for {
		var pkt *Packet
		if len(p.initial) > 0 {
			pkt, p.initial = p.initial[0], p.initial[1:]
		} else {
			_ = p.client.SetDeadline(time.Now().Add(10 * time.Second))
			var err error
			pkt, err = p.client.ReadPacket()
			if err != nil {
				return err
			}
		}

		p.Recv += len(pkt.Payload)

		// TODO: rewrite this
		var name string
		var pkt2 *core.Packet

		switch pkt.CodecID {
		case codecH264, codecH265:
			pkt2 = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      TimeToRTP(pkt.Timestamp, 90000),
				},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}
			if pkt.CodecID == codecH264 {
				name = core.CodecH264
			} else {
				name = core.CodecH265
			}
		case codecPCMA:
			name = core.CodecPCMA
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			audioTS += uint32(len(pkt.Payload))
		case codecOPUS:
			name = core.CodecOpus
			pkt2 = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      audioTS,
				},
				Payload: pkt.Payload,
			}
			// known cameras sends packets with 40ms long
			audioTS += timestamp40ms
		}

		for _, recv := range p.Receivers {
			if recv.Codec.Name == name {
				recv.WriteRTP(pkt2)
				break
			}
		}
	}
}

func (p *Producer) Stop() error {
	_ = p.client.StopMedia()
	return p.Connection.Stop()
}

// TimeToRTP convert time in milliseconds to RTP time
func TimeToRTP(timeMS, clockRate uint64) uint32 {
	return uint32(timeMS * clockRate / 1000)
}
