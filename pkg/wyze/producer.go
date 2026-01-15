package wyze

import (
	"fmt"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/wyze/tutk"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	client *Client
	model  string
}

func NewProducer(rawURL string) (*Producer, error) {
	client, err := Dial(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	// 0 = HD (default), 1 = SD/360P, 2 = 720P, 3 = 2K, 4 = Floodlight
	var quality byte
	switch s := query.Get("subtype"); s {
	case "", "hd":
		quality = 0
	case "sd":
		quality = FrameSize360P
	default:
		quality = core.ParseByte(s)
	}

	medias, err := probe(client, quality)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "wyze",
			Protocol:   client.Protocol(),
			RemoteAddr: client.RemoteAddr().String(),
			Source:     rawURL,
			Medias:     medias,
			Transport:  client,
		},
		client: client,
		model:  query.Get("model"),
	}

	return prod, nil
}

func (p *Producer) Start() error {
	for {
		if p.client.verbose {
			fmt.Println("[Wyze] Reading packet...")
		}

		_ = p.client.SetDeadline(time.Now().Add(core.ConnDeadline))
		pkt, err := p.client.ReadPacket()
		if err != nil {
			return err
		}
		if pkt == nil {
			continue
		}

		var name string
		var pkt2 *core.Packet

		switch codecID := pkt.Codec; codecID {
		case tutk.CodecH264:
			name = core.CodecH264
			pkt2 = &core.Packet{
				Header:  rtp.Header{SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}

		case tutk.CodecH265:
			name = core.CodecH265
			pkt2 = &core.Packet{
				Header:  rtp.Header{SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: annexb.EncodeToAVCC(pkt.Payload),
			}

		case tutk.AudioCodecG711U:
			name = core.CodecPCMU
			pkt2 = &core.Packet{
				Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: pkt.Payload,
			}

		case tutk.AudioCodecG711A:
			name = core.CodecPCMA
			pkt2 = &core.Packet{
				Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: pkt.Payload,
			}

		case tutk.AudioCodecAACADTS, tutk.AudioCodecAACWyze, tutk.AudioCodecAACRaw, tutk.AudioCodecAACLATM:
			name = core.CodecAAC
			payload := pkt.Payload
			if aac.IsADTS(payload) {
				payload = payload[aac.ADTSHeaderLen(payload):]
			}
			pkt2 = &core.Packet{
				Header:  rtp.Header{Version: aac.RTPPacketVersionAAC, Marker: true, SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: payload,
			}

		case tutk.AudioCodecOpus:
			name = core.CodecOpus
			pkt2 = &core.Packet{
				Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: pkt.Payload,
			}

		case tutk.AudioCodecPCM:
			name = core.CodecPCM
			pkt2 = &core.Packet{
				Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: pkt.Payload,
			}

		case tutk.AudioCodecMP3:
			name = core.CodecMP3
			pkt2 = &core.Packet{
				Header:  rtp.Header{Version: 2, Marker: true, SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: pkt.Payload,
			}

		case tutk.CodecMJPEG:
			name = core.CodecJPEG
			pkt2 = &core.Packet{
				Header:  rtp.Header{SequenceNumber: uint16(pkt.FrameNo), Timestamp: pkt.Timestamp},
				Payload: pkt.Payload,
			}

		default:
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

func probe(client *Client, quality byte) ([]*core.Media, error) {
	client.SetResolution(quality)
	client.SetDeadline(time.Now().Add(core.ProbeTimeout))

	var vcodec, acodec *core.Codec
	var tutkAudioCodec uint16

	for {
		if client.verbose {
			fmt.Println("[Wyze] Probing for codecs...")
		}

		pkt, err := client.ReadPacket()
		if err != nil {
			return nil, fmt.Errorf("wyze: probe: %w", err)
		}
		if pkt == nil || len(pkt.Payload) < 5 {
			continue
		}

		switch pkt.Codec {
		case tutk.CodecH264:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if len(buf) >= 5 && h264.NALUType(buf) == h264.NALUTypeSPS {
					vcodec = h264.AVCCToCodec(buf)
				}
			}
		case tutk.CodecH265:
			if vcodec == nil {
				buf := annexb.EncodeToAVCC(pkt.Payload)
				if len(buf) >= 5 && h265.NALUType(buf) == h265.NALUTypeVPS {
					vcodec = h265.AVCCToCodec(buf)
				}
			}
		case tutk.AudioCodecG711U:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMU, ClockRate: pkt.SampleRate, Channels: pkt.Channels}
				tutkAudioCodec = pkt.Codec
			}
		case tutk.AudioCodecG711A:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: pkt.SampleRate, Channels: pkt.Channels}
				tutkAudioCodec = pkt.Codec
			}
		case tutk.AudioCodecAACWyze, tutk.AudioCodecAACADTS, tutk.AudioCodecAACRaw, tutk.AudioCodecAACLATM:
			if acodec == nil {
				config := aac.EncodeConfig(aac.TypeAACLC, pkt.SampleRate, pkt.Channels, false)
				acodec = aac.ConfigToCodec(config)
				tutkAudioCodec = pkt.Codec
			}
		case tutk.AudioCodecOpus:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecOpus, ClockRate: 48000, Channels: 2}
				tutkAudioCodec = pkt.Codec
			}
		case tutk.AudioCodecPCM:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCM, ClockRate: pkt.SampleRate, Channels: pkt.Channels}
				tutkAudioCodec = pkt.Codec
			}
		case tutk.AudioCodecMP3:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecMP3, ClockRate: pkt.SampleRate, Channels: pkt.Channels}
				tutkAudioCodec = pkt.Codec
			}
		case tutk.CodecMJPEG:
			if vcodec == nil {
				vcodec = &core.Codec{Name: core.CodecJPEG, ClockRate: 90000, PayloadType: core.PayloadTypeRAW}
			}
		}

		if vcodec != nil && (acodec != nil || !client.SupportsAudio()) {
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

		if client.SupportsIntercom() {
			client.SetBackchannelCodec(tutkAudioCodec, acodec.ClockRate, uint8(acodec.Channels))
			medias = append(medias, &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionSendonly,
				Codecs:    []*core.Codec{acodec.Clone()},
			})
		}
	}

	if client.verbose {
		fmt.Printf("[Wyze] Probed codecs: video=%s audio=%s\n", vcodec.Name, acodec.Name)
		if client.SupportsIntercom() {
			fmt.Printf("[Wyze] Intercom supported, audio send codec=%s\n", acodec.Name)
		}
	}

	return medias, nil
}
