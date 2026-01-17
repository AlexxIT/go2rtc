package legacy

import (
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/rtp"
)

func Dial(rawURL string) (*Producer, error) {
	client, err := NewClient(rawURL)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(rawURL)
	query := u.Query()

	err = client.StartMedia(query.Get("subtype"), "")
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	medias, err := probe(client)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	c := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "xiaomi/legacy",
			Protocol:   "tutk+udp",
			RemoteAddr: client.RemoteAddr().String(),
			UserAgent:  client.Version(),
			Medias:     medias,
			Transport:  client,
		},
		client: client,
	}
	return c, nil
}

type Producer struct {
	core.Connection
	client *Client
}

const codecXiaobaiPCMA = 1 // chuangmi.camera.xiaobai

func probe(client *Client) ([]*core.Media, error) {
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))

	var vcodec, acodec *core.Codec

	for {
		// 0   5000      codec
		// 2   0000      codec params
		// 4   01        active clients
		// 5   34        unknown const
		// 6   0600      unknown seq(s)
		// 8   80026801  unknown fixed
		// 12  ed8d5c69  time in sec
		// 16  4c03      time in 1/1000
		// 18  0000
		hdr, payload, err := client.ReadPacket()
		if err != nil {
			return nil, err
		}

		switch codec := hdr[0]; codec {
		case tutk.CodecH264, tutk.CodecH265:
			if vcodec == nil {
				avcc := annexb.EncodeToAVCC(payload)
				if codec == tutk.CodecH264 {
					if h264.NALUType(avcc) == h264.NALUTypeSPS {
						vcodec = h264.AVCCToCodec(avcc)
					}
				} else {
					if h265.NALUType(avcc) == h265.NALUTypeVPS {
						vcodec = h265.AVCCToCodec(avcc)
					}
				}
			}
		case tutk.CodecPCMA, codecXiaobaiPCMA:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCMA, ClockRate: 8000}
			}
		case tutk.CodecPCML:
			if acodec == nil {
				acodec = &core.Codec{Name: core.CodecPCML, ClockRate: 8000}
			}
		case tutk.CodecAACLATM:
			if acodec == nil {
				acodec = aac.ADTSToCodec(payload)
				if acodec != nil {
					acodec.PayloadType = core.PayloadTypeRAW
				}
			}
		}

		if vcodec != nil && acodec != nil {
			break
		}
	}

	medias := []*core.Media{
		{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{vcodec},
		},
		{
			Kind:      core.KindAudio,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{acodec},
		},
	}
	return medias, nil
}

func (c *Producer) Protocol() string {
	return "tutk+udp"
}

func (c *Producer) Start() error {
	var audioTS uint32
	var videoSeq, audioSeq uint16

	for {
		_ = c.client.SetDeadline(time.Now().Add(5 * time.Second))
		hdr, payload, err := c.client.ReadPacket()
		if err != nil {
			return err
		}

		n := len(payload)
		c.Recv += n

		// TODO: rewrite this
		var name string
		var pkt *core.Packet

		switch codec := hdr[0]; codec {
		case tutk.CodecH264, tutk.CodecH265:
			pkt = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: videoSeq,
					Timestamp:      core.Now90000(),
				},
				Payload: annexb.EncodeToAVCC(payload),
			}
			videoSeq++

			if codec == tutk.CodecH264 {
				name = core.CodecH264
			} else {
				name = core.CodecH265
			}

		case tutk.CodecPCMA, tutk.CodecPCML, codecXiaobaiPCMA:
			pkt = &core.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: audioSeq,
					Timestamp:      audioTS,
				},
				Payload: payload,
			}
			audioSeq++

			switch codec {
			case tutk.CodecPCMA, codecXiaobaiPCMA:
				name = core.CodecPCMA
				audioTS += uint32(n)
			case tutk.CodecPCML:
				name = core.CodecPCML
				audioTS += uint32(n / 2) // because 16bit
			}

		case tutk.CodecAACLATM:
			pkt = &core.Packet{
				Header: rtp.Header{
					SequenceNumber: audioSeq,
					Timestamp:      audioTS,
				},
				Payload: payload,
			}
			audioSeq++

			name = core.CodecAAC
			audioTS += 1024
		}

		for _, recv := range c.Receivers {
			if recv.Codec.Name == name {
				recv.WriteRTP(pkt)
				break
			}
		}
	}
}

func (c *Producer) Stop() error {
	_ = c.client.StopMedia()
	return c.Connection.Stop()
}
