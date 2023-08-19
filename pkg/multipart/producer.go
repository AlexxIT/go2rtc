package multipart

import (
	"bufio"
	"errors"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

type Producer struct {
	core.SuperProducer
	rd *core.ReadBuffer

	boundary string
	reader   *bufio.Reader
}

func Open(rd io.Reader) (*Producer, error) {
	prod := &Producer{rd: core.NewReadBuffer(rd)}
	if err := prod.probe(); err != nil {
		return nil, err
	}
	prod.Type = "Multipart producer"
	return prod, nil
}

func (c *Producer) Start() error {
	if len(c.Receivers) == 0 {
		return errors.New("multipart: no receivers")
	}

	var mjpeg, video, audio *core.Receiver

	for _, receiver := range c.Receivers {
		switch receiver.Codec.Name {
		case core.CodecH264:
			video = receiver
		case core.CodecPCMU:
			audio = receiver
		default:
			mjpeg = receiver
		}
	}

	for {
		header, body, err := c.next()
		if err != nil {
			return err
		}

		c.Recv += len(body)

		if mjpeg != nil {
			packet := &rtp.Packet{
				Header:  rtp.Header{Timestamp: core.Now90000()},
				Payload: body,
			}
			mjpeg.WriteRTP(packet)
			continue
		}

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

func (c *Producer) next() (http.Header, []byte, error) {
	for {
		// search next boundary and skip empty lines
		s, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, nil, err
		}

		// auto guess boundary
		if c.boundary == "" && strings.HasPrefix(s, "--") {
			c.boundary = s
			break
		} else if strings.HasPrefix(s, c.boundary) {
			break
		}

		if s == "\r\n" {
			continue
		}

		return nil, nil, errors.New("multipart: wrong boundary: " + s)
	}

	tp := textproto.NewReader(c.reader)
	header, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, nil, err
	}

	s := header.Get("Content-Length")
	if s == "" {
		return nil, nil, errors.New("multipart: no content length")
	}

	size, err := strconv.Atoi(s)
	if err != nil {
		return nil, nil, err
	}

	buf := make([]byte, size)
	if _, err = io.ReadFull(c.reader, buf); err != nil {
		return nil, nil, err
	}

	_, _ = c.reader.Discard(2) // skip "\r\n"

	return http.Header(header), buf, nil
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

	waitVideo := true
	waitAudio := true

	for waitVideo || waitAudio {
		header, _, err := c.next()
		if err != nil {
			return err
		}

		var media *core.Media

		ct := header.Get("Content-Type")
		switch ct {
		case MimeVideo:
			if !waitVideo {
				return nil
			}

			media = &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{
						Name:        core.CodecH264,
						ClockRate:   90000,
						PayloadType: core.PayloadTypeRAW,
					},
				},
			}
			waitVideo = false

		case MimeG711U:
			if !waitAudio {
				return nil
			}

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
			waitAudio = false

		default:
			media = &core.Media{
				Kind:      core.KindVideo,
				Direction: core.DirectionRecvonly,
				Codecs: []*core.Codec{
					{
						Name:        core.CodecJPEG,
						ClockRate:   90000,
						PayloadType: core.PayloadTypeRAW,
					},
				},
			}
			waitVideo = false
			waitAudio = false
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
