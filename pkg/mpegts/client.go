package mpegts

import (
	"bytes"
	"io"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
)

type Client struct {
	URL string

	rd *core.ReadBuffer

	medias    []*core.Media
	receivers []*core.Receiver

	recv int
}

func Open(rd io.Reader) (*Client, error) {
	client := &Client{rd: core.NewReadBuffer(rd)}
	if err := client.describe(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *Client) describe() error {
	c.rd.BufferSize = core.ProbeSize
	defer c.rd.Reset()

	rd := NewReader()

	// Strategy:
	// 1. Wait packet with metadata, init other packets for wait
	// 2. Wait other packets
	// 3. Stop after timeout
	waitType := []byte{metadataType}
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
		case metadataType:
			for _, streamType := range pkt.Payload {
				switch streamType {
				case StreamTypeH264, StreamTypeH265, StreamTypeAAC:
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
			c.medias = append(c.medias, media)

		case StreamTypeH265:
			codec := h265.AVCCToCodec(pkt.Payload)
			media := &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)

		case StreamTypeAAC:
			codec := aac.RTPToCodec(pkt.Payload)
			media := &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}
			c.medias = append(c.medias, media)
		}
	}

	return nil
}

func (c *Client) play() error {
	rd := NewReader()

	for {
		pkt, err := rd.ReadPacket(c.rd)
		if err != nil {
			return err
		}

		//log.Printf("[mpegts] size: %6d, ts: %10d, pt: %2d", len(pkt.Payload), pkt.Timestamp, pkt.PayloadType)

		for _, receiver := range c.receivers {
			if receiver.ID == pkt.PayloadType {
				pkt.Timestamp = PTSToTimestamp(pkt.Timestamp, receiver.Codec.ClockRate)
				receiver.WriteRTP(pkt)
				break
			}
		}
	}
}

func (c *Client) Close() error {
	if closer, ok := c.rd.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
