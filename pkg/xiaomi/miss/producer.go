package miss

import (
	"fmt"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	client *Client
	lenses *lensOutputs
}

func Dial(rawURL string) (core.Producer, error) {
	p, err := dial(rawURL, false)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func dial(rawURL string, dual bool) (*Producer, error) {
	client, err := NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	channel := query.Get("channel")
	videoCount := 1
	if dual {
		videoCount = 2
	}
	err = client.startMedia(channel, query.Get("subtype"), query.Get("audio"), dual)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	medias, err := probe(client, query.Get("audio") != "0", videoCount)
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
		client: client,
	}, nil
}

func probe(client *Client, audio bool, videoCount int) ([]*core.Media, error) {
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))

	vcodecs := make([]*core.Codec, videoCount)
	var acodec *core.Codec
	var videos int

	for {
		pkt, err := client.ReadPacket()
		if err != nil {
			if videos == videoCount {
				err = fmt.Errorf("no audio")
			} else if videos != 0 {
				err = fmt.Errorf("missing video channel")
			} else if acodec != nil {
				err = fmt.Errorf("no video")
			}
			return nil, fmt.Errorf("xiaomi: probe: %w", err)
		}

		channel := 0
		if videoCount > 1 {
			channel = int(pkt.videoChannel())
		}
		switch pkt.CodecID {
		case codecH264:
			if vcodecs[channel] == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if len(buf) > 4 && h264.NALUType(buf) == h264.NALUTypeSPS {
					vcodecs[channel] = h264.AVCCToCodec(buf)
					videos++
				}
			}
		case codecH265:
			if vcodecs[channel] == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if len(buf) > 4 && h265.NALUType(buf) == h265.NALUTypeVPS {
					vcodecs[channel] = h265.AVCCToCodec(buf)
					videos++
				}
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

		if videos == videoCount && (acodec != nil || !audio) {
			break
		}
	}

	_ = client.SetDeadline(time.Time{})

	medias := make([]*core.Media, 0, videoCount+2)
	for _, codec := range vcodecs {
		media := &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{codec},
		}
		medias = append(medias, media)
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

	return medias, nil
}

const timestamp40ms = 48000 * 0.040

func (p *Producer) Start() error {
	var audioTS uint32

	for {
		_ = p.client.SetDeadline(time.Now().Add(10 * time.Second))
		pkt, err := p.client.ReadPacket()
		if err != nil {
			// Free the camera slot now: streams stops this producer only after
			// the reconnect succeeds, and the reconnect may need that slot.
			_ = p.client.Close()
			return err
		}

		p.Recv += len(pkt.Payload)

		// TODO: rewrite this
		var name string
		var pkt2 *core.Packet

		switch pkt.CodecID {
		case codecH264, codecH265:
			if p.lenses != nil && p.lenses[pkt.videoChannel()].Load() == nil {
				continue
			}
			payload := annexb.EncodeToAVCC(pkt.Payload)
			if len(payload) <= 4 {
				continue
			}
			pkt2 = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: uint16(pkt.Sequence),
					Timestamp:      TimeToRTP(pkt.Timestamp, 90000),
				},
				Payload: payload,
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

		if p.lenses != nil {
			p.lenses.write(pkt, name, pkt2)
			continue
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
