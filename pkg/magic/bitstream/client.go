package bitstream

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
)

type Client struct {
	rd *core.ReadSeeker

	media    *core.Media
	receiver *core.Receiver

	recv int
}

func Open(r io.Reader) (*Client, error) {
	rd := core.NewReadSeeker(r)

	buf, err := rd.Peek(256)
	if err != nil {
		return nil, err
	}

	buf = annexb.EncodeToAVCC(buf, false) // won't break original buffer

	var codec *core.Codec

	switch {
	case h264.NALUType(buf) == h264.NALUTypeSPS:
		codec = h264.AVCCToCodec(buf)
	case h265.NALUType(buf) == h265.NALUTypeVPS:
		codec = h265.AVCCToCodec(buf)
	default:
		return nil, errors.New("bitstream: unsupported header: " + hex.EncodeToString(buf[:8]))
	}

	client := &Client{
		rd: rd,
		media: &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{codec},
		},
	}

	return client, nil
}

func (c *Client) GetMedias() []*core.Media {
	return []*core.Media{c.media}
}

func (c *Client) GetTrack(media *core.Media, codec *core.Codec) (*core.Receiver, error) {
	if c.receiver == nil {
		c.receiver = core.NewReceiver(media, codec)
	}
	return c.receiver, nil
}

func (c *Client) Start() error {
	var buf []byte

	b := make([]byte, core.BufferSize)
	for {
		n, err := c.rd.Read(b)
		if err != nil {
			return err
		}

		c.recv += n

		buf = append(buf, b[:n]...)

		i := annexb.IndexFrame(buf)
		if i < 0 {
			continue
		}

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: annexb.EncodeToAVCC(buf[:i], true),
		}
		c.receiver.WriteRTP(pkt)

		//log.Printf("[AVC] %v, len: %d", h264.Types(pkt.Payload), len(pkt.Payload))

		buf = buf[i:]
	}
}

func (c *Client) Stop() error {
	if c.receiver != nil {
		c.receiver.Close()
	}
	if closer, ok := c.rd.Reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func (c *Client) MarshalJSON() ([]byte, error) {
	info := &core.Info{
		Type:      "Bitstream active producer",
		Medias:    []*core.Media{c.media},
		Receivers: []*core.Receiver{c.receiver},
		Recv:      c.recv,
	}
	return json.Marshal(info)
}
