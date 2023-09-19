package kasa

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"strconv"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/multipart"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

type Producer struct {
	core.SuperProducer
	rd *core.ReadBuffer

	reader *bufio.Reader
}

func Dial(url string) (*Producer, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.URL.Scheme = "httpx"

	res, err := tcp.Do(req)
	if err != nil {
		return nil, err
	}

	rd := struct {
		io.Reader
		io.Closer
	}{
		httputil.NewChunkedReader(res.Body),
		res.Body,
	}

	prod := &Producer{rd: core.NewReadBuffer(rd)}
	if err = prod.probe(); err != nil {
		return nil, err
	}
	prod.Type = "Kasa producer"
	return prod, nil
}

func (c *Producer) Start() error {
	if len(c.Receivers) == 0 {
		return errors.New("multipart: no receivers")
	}

	var video, audio *core.Receiver

	for _, receiver := range c.Receivers {
		switch receiver.Codec.Name {
		case core.CodecH264:
			video = receiver
		case core.CodecPCMU:
			audio = receiver
		}
	}

	for {
		header, body, err := multipart.Next(c.reader)
		if err != nil {
			return err
		}

		c.Recv += len(body)

		ct := header.Get("Content-Type")
		switch ct {
		case MimeVideo:
			if video != nil {
				ts := GetTimestamp(header)
				pkt := &rtp.Packet{
					Header: rtp.Header{
						Timestamp: uint32(ts * 90000),
					},
					Payload: annexb.EncodeToAVCC(body, false),
				}
				video.WriteRTP(pkt)
			}

		case MimeG711U:
			if audio != nil {
				ts := GetTimestamp(header)
				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:   2,
						Marker:    true,
						Timestamp: uint32(ts * 8000),
					},
					Payload: body,
				}
				audio.WriteRTP(pkt)
			}
		}
	}
}

func (c *Producer) Stop() error {
	_ = c.SuperProducer.Close()
	return c.rd.Close()
}

const (
	MimeVideo = "video/x-h264"
	MimeG711U = "audio/g711u"
)

func (c *Producer) probe() error {
	c.rd.BufferSize = core.ProbeSize
	c.reader = bufio.NewReader(c.rd)

	defer func() {
		c.rd.Reset()
		c.reader = bufio.NewReader(c.rd)
	}()

	waitVideo, waitAudio := true, true
	timeout := time.Now().Add(core.ProbeTimeout)

	for (waitVideo || waitAudio) && time.Now().Before(timeout) {
		header, body, err := multipart.Next(c.reader)
		if err != nil {
			return err
		}

		var media *core.Media

		ct := header.Get("Content-Type")
		switch ct {
		case MimeVideo:
			if !waitVideo {
				continue
			}
			waitVideo = false

			body = annexb.EncodeToAVCC(body, false)
			codec := h264.AVCCToCodec(body)
			media = &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs:    []*core.Codec{codec},
			}

		case MimeG711U:
			if !waitAudio {
				continue
			}
			waitAudio = false

			media = &core.Media{
				Kind:      core.KindAudio,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{
						Name:      core.CodecPCMU,
						ClockRate: 8000,
					},
				},
			}

		default:
			return errors.New("kasa: unsupported type: " + ct)
		}

		c.Medias = append(c.Medias, media)
	}

	return nil
}

// GetTimestamp - return timestamp in seconds
func GetTimestamp(header http.Header) float64 {
	if s := header.Get("X-Timestamp"); s != "" {
		if f, _ := strconv.ParseFloat(s, 32); f != 0 {
			return f
		}
	}

	return float64(time.Duration(time.Now().UnixNano()) / time.Second)
}
