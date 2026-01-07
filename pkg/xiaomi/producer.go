package xiaomi

import (
	"errors"
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
	stream *stream
	model  string
}

func Dial(rawURL string) (core.Producer, error) {
	sess, err := getSession(rawURL)
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

	st := sess.openStream(channel)
	medias, err := probe(st, quality)
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi",
			Protocol:   "cs2+udp",
			RemoteAddr: st.RemoteAddr().String(),
			Source:     rawURL,
			Medias:     medias,
			Transport:  st,
		},
		stream: st,
		model:  query.Get("model"),
	}, nil
}

func probe(st *stream, quality uint8) ([]*core.Media, error) {
	_ = st.SetDeadline(time.Now().Add(core.ProbeTimeout))

	if err := st.VideoStart(quality, 1); err != nil {
		return nil, err
	}

	var video, audio *core.Codec
	needAudio := st.wantsAudio()

	for {
		pkt, err := st.ReadPacket()
		if err != nil {
			if errors.Is(err, errTimeout) && video != nil {
				break
			}
			return nil, fmt.Errorf("xiaomi: probe: %w", err)
		}

		switch pkt.CodecID {
		case miss.CodecH264:
			if video == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h264.NALUType(buf) == h264.NALUTypeSPS {
					video = h264.AVCCToCodec(buf)
				}
			}
		case miss.CodecH265:
			if video == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if h265.NALUType(buf) == h265.NALUTypeVPS {
					video = h265.AVCCToCodec(buf)
				}
			}
		case miss.CodecPCMA:
			if audio == nil {
				audio = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
			}
		case miss.CodecOPUS:
			if audio == nil {
				audio = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
			}
		}

		if video != nil && (audio != nil || !needAudio) {
			break
		}
	}

	_ = st.SetDeadline(time.Time{})

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{video},
		},
	}

	if audio != nil {
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{audio},
		})
		medias = append(medias, &core.Media{
			Kind:      core.KindAudio,
			Direction: core.DirectionSendonly,
			Codecs:    []*core.Codec{audio.Clone()},
		})
	}

	return medias, nil
}

const timestamp40ms = 48000 * 0.040

func (p *Producer) Start() error {
	var audioTS uint32

	for {
		_ = p.stream.SetDeadline(time.Now().Add(core.ConnDeadline))
		pkt, err := p.stream.ReadPacket()
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
