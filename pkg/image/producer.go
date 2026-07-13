package image

import (
	"bytes"
	"errors"
	"image/jpeg"
	"io"
	"net/http"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	webp "github.com/skrashevich/go-webp"
)

type Producer struct {
	core.Connection

	closed bool
	res    *http.Response
}

func Open(res *http.Response) (*Producer, error) {
	return &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "image",
			Protocol:   "http",
			RemoteAddr: res.Request.URL.Host,
			Transport:  res.Body,
			Medias: []*core.Media{
				{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs: []*core.Codec{
						{
							Name:        core.CodecJPEG,
							ClockRate:   90000,
							PayloadType: core.PayloadTypeRAW,
						},
					},
				},
			},
		},
		res: res,
	}, nil
}

func (c *Producer) Start() error {
	body, err := io.ReadAll(c.res.Body)
	if err != nil {
		return err
	}

	if isWebP(body) {
		if converted, err2 := webpToJPEG(body); err2 == nil {
			body = converted
		}
	}

	pkt := &rtp.Packet{
		Header:  rtp.Header{Timestamp: core.Now90000()},
		Payload: body,
	}
	c.Receivers[0].WriteRTP(pkt)

	c.Recv += len(body)

	req := c.res.Request

	for !c.closed {
		res, err := tcp.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode != http.StatusOK {
			return errors.New("wrong status: " + res.Status)
		}

		body, err = io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		if isWebP(body) {
			if converted, err2 := webpToJPEG(body); err2 == nil {
				body = converted
			}
		}

		c.Recv += len(body)

		pkt = &rtp.Packet{
			Header:  rtp.Header{Timestamp: core.Now90000()},
			Payload: body,
		}
		c.Receivers[0].WriteRTP(pkt)
	}

	return nil
}

func (c *Producer) Stop() error {
	c.closed = true
	return c.Connection.Stop()
}

// isWebP returns true if data starts with RIFF....WEBP magic bytes.
func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P'
}

// webpToJPEG decodes WebP bytes and re-encodes as JPEG.
func webpToJPEG(data []byte) ([]byte, error) {
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err = jpeg.Encode(&buf, img, nil); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
