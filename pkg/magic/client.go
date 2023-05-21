package magic

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/pion/rtp"
	"io"
)

// Client - can read unknown bytestream and autodetect format
type Client struct {
	Desc string
	URL  string

	Handle func() error

	r     io.ReadCloser
	sniff []byte

	medias   []*core.Media
	receiver *core.Receiver

	recv int
}

func NewClient(r io.ReadCloser) *Client {
	return &Client{r: r}
}

func (c *Client) Probe() (err error) {
	c.sniff = make([]byte, mpegts.PacketSize*3) // MPEG-TS: SDT+PAT+PMT
	c.recv, err = io.ReadFull(c.r, c.sniff)
	if err != nil {
		_ = c.Close()
		return
	}

	var codec *core.Codec

	if bytes.HasPrefix(c.sniff, []byte{0, 0, 0, 1}) {
		switch {
		case h264.NALUType(c.sniff) == h264.NALUTypeSPS:
			codec = &core.Codec{
				Name:        core.CodecH264,
				ClockRate:   90000,
				PayloadType: core.PayloadTypeRAW,
			}
			c.Handle = c.ReadBitstreams

		case h265.NALUType(c.sniff) == h265.NALUTypeVPS:
			codec = &core.Codec{
				Name:        core.CodecH265,
				ClockRate:   90000,
				PayloadType: core.PayloadTypeRAW,
			}
			c.Handle = c.ReadBitstreams
		}

	} else if bytes.HasPrefix(c.sniff, []byte{0xFF, 0xD8}) {
		codec = &core.Codec{
			Name:        core.CodecJPEG,
			ClockRate:   90000,
			PayloadType: core.PayloadTypeRAW,
		}
		c.Handle = c.ReadMJPEG

	} else if c.sniff[0] == mpegts.SyncByte {
		ts := mpegts.NewReader()
		ts.AppendBuffer(c.sniff)
		_ = ts.GetPacket()
		for _, streamType := range ts.GetStreamTypes() {
			switch streamType {
			case mpegts.StreamTypeH264:
				codec = &core.Codec{
					Name:        core.CodecH264,
					ClockRate:   90000,
					PayloadType: core.PayloadTypeRAW,
				}
				c.Handle = c.ReadMPEGTS

			case mpegts.StreamTypeH265:
				codec = &core.Codec{
					Name:        core.CodecH265,
					ClockRate:   90000,
					PayloadType: core.PayloadTypeRAW,
				}
				c.Handle = c.ReadMPEGTS
			}
		}
	}

	if codec == nil {
		_ = c.Close()
		return errors.New("unknown format: " + hex.EncodeToString(c.sniff[:8]))
	}

	c.medias = append(c.medias, &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{codec},
	})

	return
}

func (c *Client) ReadBitstreams() error {
	buf := c.sniff               // total bufer
	b := make([]byte, 1024*1024) // reading buffer

	var decodeStream func([]byte) ([]byte, int)
	switch c.receiver.Codec.Name {
	case core.CodecH264:
		decodeStream = h264.DecodeStream
	case core.CodecH265:
		decodeStream = h265.DecodeStream
	}

	for {
		payload, n := decodeStream(buf)
		if payload == nil {
			n, err := c.r.Read(b)
			if err != nil {
				return err
			}

			buf = append(buf, b[:n]...)
			c.recv += n
			continue
		}

		buf = buf[n:]

		//log.Printf("[AVC] %v, len: %d", h264.Types(payload), len(payload))

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: payload,
		}
		c.receiver.WriteRTP(pkt)
	}
}

func (c *Client) ReadMJPEG() error {
	buf := c.sniff               // total bufer
	b := make([]byte, 1024*1024) // reading buffer

	for {
		// one JPEG end and next start
		i := bytes.Index(buf, []byte{0xFF, 0xD9, 0xFF, 0xD8})
		if i < 0 {
			n, err := c.r.Read(b)
			if err != nil {
				return err
			}

			buf = append(buf, b[:n]...)
			c.recv += n

			// if we receive frame
			if n >= 2 && b[n-2] == 0xFF && b[n-1] == 0xD9 {
				i = len(buf)
			} else {
				continue
			}
		} else {
			i += 2
		}

		pkt := &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: buf[:i],
		}
		c.receiver.WriteRTP(pkt)

		buf = buf[i:]
	}
}

func (c *Client) ReadMPEGTS() error {
	b := make([]byte, 1024*1024) // reading buffer

	ts := mpegts.NewReader()
	ts.AppendBuffer(c.sniff)

	for {
		packet := ts.GetPacket()
		if packet == nil {
			n, err := c.r.Read(b)
			if err != nil {
				return err
			}

			ts.AppendBuffer(b[:n])
			c.recv += n
			continue
		}

		//log.Printf("[AVC] %v, len: %d, ts: %10d", h264.Types(packet.Payload), len(packet.Payload), packet.Timestamp)

		switch packet.PayloadType {
		case mpegts.StreamTypeH264, mpegts.StreamTypeH265:
			c.receiver.WriteRTP(packet)
		}
	}
}

func (c *Client) Close() error {
	return c.r.Close()
}
