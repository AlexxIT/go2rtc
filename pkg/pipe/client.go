package pipe

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/mpegts"
	"github.com/pion/rtp"
	"io"
	"os/exec"
)

type Client struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	sniff  []byte
	handle func() error

	medias   []*core.Media
	receiver *core.Receiver

	recv int
}

func NewClient(cmd *exec.Cmd) (prod *Client, err error) {
	prod = &Client{cmd: cmd}

	prod.stdout, err = cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	prod.sniff = make([]byte, mpegts.PacketSize*3) // MPEG-TS: SDT+PAT+PMT
	prod.recv, err = io.ReadFull(prod.stdout, prod.sniff)
	if err != nil {
		_ = prod.Stop()
		return nil, err
	}

	var codec *core.Codec

	if bytes.HasPrefix(prod.sniff, []byte{0, 0, 0, 1}) {
		switch {
		case h264.NALUType(prod.sniff) == h264.NALUTypeSPS:
			codec = &core.Codec{
				Name:        core.CodecH264,
				ClockRate:   90000,
				PayloadType: core.PayloadTypeRAW,
			}
			prod.handle = prod.ReadBitstreams
		}
	} else if bytes.HasPrefix(prod.sniff, []byte{0xFF, 0xD8}) {
		codec = &core.Codec{
			Name:        core.CodecJPEG,
			ClockRate:   90000,
			PayloadType: core.PayloadTypeRAW,
		}
		prod.handle = prod.ReadMJPEG
	} else if prod.sniff[0] == mpegts.SyncByte {
		ts := mpegts.NewReader()
		ts.AppendBuffer(prod.sniff)
		_ = ts.GetPacket()
		for _, streamType := range ts.GetStreamTypes() {
			switch streamType {
			case mpegts.StreamTypeH264:
				codec = &core.Codec{
					Name:        core.CodecH264,
					ClockRate:   90000,
					PayloadType: core.PayloadTypeRAW,
				}
				prod.handle = prod.ReadMPEGTS
			}
		}
	}

	if codec == nil {
		_ = prod.Stop()
		return nil, errors.New("unknown format: " + hex.EncodeToString(prod.sniff))
	}

	prod.medias = append(prod.medias, &core.Media{
		Kind:      core.KindVideo,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{codec},
	})

	return
}

func (c *Client) ReadBitstreams() error {
	buf := c.sniff               // total bufer
	b := make([]byte, 1024*1024) // reading buffer

	for {
		payload, n := h264.DecodeStream(buf)
		if payload == nil {
			n, err := c.stdout.Read(b)
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
			n, err := c.stdout.Read(b)
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
			n, err := c.stdout.Read(b)
			if err != nil {
				return err
			}

			ts.AppendBuffer(b[:n])
			c.recv += n
			continue
		}

		//log.Printf("[AVC] %v, len: %d, ts: %10d", h264.Types(packet.Payload), len(packet.Payload), packet.Timestamp)

		if packet.PayloadType != mpegts.StreamTypeH264 {
			continue
		}

		c.receiver.WriteRTP(packet)
	}
}
