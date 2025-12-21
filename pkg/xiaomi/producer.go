package xiaomi

import (
	"fmt"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	client *miss.Client
	model  string
}

func Dial(rawURL string) (core.Producer, error) {
	client, err := miss.Dial(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	// 0 - main, 1 - second
	channel := core.ParseByte(query.Get("channel"))

	// 0 - auto, 1 - worst, 3 or 5 - best
	var quality byte
	switch s := query.Get("subtype"); s {
	case "", "hd":
		quality = 3
	case "sd":
		quality = 1
	case "auto":
		quality = 0
	default:
		quality = core.ParseByte(s)
	}

	// 0 - disabled, 1 - enabled, 2 - enabled (another API)
	var audio byte
	switch s := query.Get("audio"); s {
	case "", "1":
		audio = 1
	default:
		audio = core.ParseByte(s)
	}

	medias, err := probe(client, channel, quality, audio)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi",
			Protocol:   client.Protocol(),
			RemoteAddr: client.RemoteAddr().String(),
			Source:     rawURL,
			Medias:     medias,
			Transport:  client,
		},
		client: client,
		model:  query.Get("model"),
	}, nil
}

func probe(client *miss.Client, channel, quality, audio uint8) ([]*core.Media, error) {
	_ = client.SetDeadline(time.Now().Add(core.ProbeTimeout))

	if err := client.VideoStart(channel, quality, audio&1); err != nil {
		return nil, err
	}

	if audio > 1 {
		_ = client.AudioStart()
	}

	var vcodec, acodec *core.Codec

	for {
		pkt, err := client.ReadPacket()
		if err != nil {
			return nil, fmt.Errorf("xiaomi: probe: %w", err)
		}

		switch pkt.CodecID {
		case miss.CodecH264:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h264.NALUType(buf) == h264.NALUTypeSPS {
					vcodec = h264.AVCCToCodec(buf)
				}
			}
		case miss.CodecH265:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h265.NALUType(buf) == h265.NALUTypeVPS {
					vcodec = h265.AVCCToCodec(buf)
				}
			}
		case miss.CodecPCMA:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
			}
		case miss.CodecOPUS:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
			}
		}

		if vcodec != nil && (acodec != nil || audio == 0) {
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

		if client.Protocol() == "cs2+udp" {
			medias = append(medias, &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{acodec.Clone()},
			})
		}
	}

	return medias, nil
}

const timestamp40ms = 48000 * 0.040

func (p *Producer) Start() error {
	var audioTS uint32

	for {
		_ = p.client.SetDeadline(time.Now().Add(core.ConnDeadline))
		pkt, err := p.client.ReadPacket()
		if err != nil {
			return err
		}

		// TODO: rewrite this
		var name string
		var pkt2 *core.Packet

		switch pkt.CodecID {
		case miss.CodecH264:
			name = core.CodecH264
			pkt2 = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      TimeToRTP(pkt.Timestamp, 90000),
				},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}
		case miss.CodecH265:
			name = core.CodecH265
			pkt2 = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      TimeToRTP(pkt.Timestamp, 90000),
				},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}
		case miss.CodecPCMA:
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
		case miss.CodecOPUS:
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

// TimeToRTP convert time in milliseconds to RTP time
func TimeToRTP(timeMS, clockRate uint64) uint32 {
	return uint32(timeMS * clockRate / 1000)
}
