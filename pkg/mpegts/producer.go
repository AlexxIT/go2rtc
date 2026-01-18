package mpegts

import (
	"bytes"
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	rd *core.ReadBuffer
}

func Open(rd io.Reader) (*Producer, error) {
	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "mpegts",
			Transport:  rd,
		},
		rd: core.NewReadBuffer(rd),
	}
	if err := prod.probe(); err != nil {
		return nil, err
	}
	return prod, nil
}

func (c *Producer) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	receiver, _ := c.Connection.GetTrack(media, codec)
	receiver.ID = StreamType(codec)
	return receiver, nil
}

func (c *Producer) Start() error {
	rd := NewDemuxer()

	for {
		pkt, err := rd.ReadPacket(c.rd)
		if err != nil {
			return err
		}

		c.Recv += len(pkt.Payload)

		//log.Printf("[mpegts] size: %6d, muxer: %10d, pt: %2d", len(pkt.Payload), pkt.Timestamp, pkt.PayloadType)

		for _, receiver := range c.Receivers {
			if receiver.ID == pkt.PayloadType {
				TimestampToRTP(pkt, receiver.Codec)
				receiver.WriteRTP(pkt)
				break
			}
		}
	}
}

func (c *Producer) probe() error {
	c.rd.BufferSize = core.ProbeSize
	defer c.rd.Reset()

	rd := NewDemuxer()

	// Strategy:
	// 1. Wait packet with metadata, init other packets for wait
	// 2. Wait other packets
	// 3. Stop after timeout
	waitType := []byte{StreamTypeMetadata}
	timeout := time.Now().Add(core.ProbeTimeout)

	for len(waitType) != 0 && time.Now().Before(timeout) {
		pkt, err := rd.ReadPacket(c.rd)
		if err != nil {
			return err
		}

		// check if we wait this type
		if i := bytes.IndexByte(waitType, pkt.PayloadType); i < 0 {
			continue
		} else {
			waitType = append(waitType[:i], waitType[i+1:]...)
		}

		switch pkt.PayloadType {
		case StreamTypeMetadata:
			for _, streamType := range pkt.Payload {
				switch streamType {
				case StreamTypeH264, StreamTypeH265, StreamTypeAAC, StreamTypePrivateOPUS:
					waitType = append(waitType, streamType)
				}
			}

		case StreamTypeH264:
			codec := h264.AVCCToCodec(pkt.Payload)
			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)

		case StreamTypeH265:
			codec := h265.AVCCToCodec(pkt.Payload)
			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)

		case StreamTypeAAC:
			codec := aac.RTPToCodec(pkt.Payload)
			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)

		case StreamTypePrivateOPUS:
			codec := &core.Codec{
				Name:      core.CodecOpus,
				ClockRate: 48000,
				Channels:  2,
			}
			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.Medias = append(c.Medias, media)
		}
	}

	return nil
}

func StreamType(codec *core.Codec) uint8 {
	switch codec.Name {
	case core.CodecH264:
		return StreamTypeH264
	case core.CodecH265:
		return StreamTypeH265
	case core.CodecAAC:
		return StreamTypeAAC
	case core.CodecPCMA:
		return StreamTypePCMATapo
	case core.CodecOpus:
		return StreamTypePrivateOPUS
	}
	return 0
}

func TimestampToRTP(rtp *rtp.Packet, codec *core.Codec) {
	if codec.ClockRate == ClockRate {
		return
	}
	rtp.Timestamp = uint32(float64(rtp.Timestamp) * float64(codec.ClockRate) / ClockRate)
}
