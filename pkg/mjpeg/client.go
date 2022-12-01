package mjpeg

import (
	"bufio"
	"errors"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
)

type Client struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	closed bool
	res    *http.Response

	track *streamer.Track
}

func NewClient(res *http.Response) *Client {
	codec := &streamer.Codec{
		Name: streamer.CodecJPEG, ClockRate: 90000, PayloadType: streamer.PayloadTypeRAW,
	}
	return &Client{
		res:   res,
		track: streamer.NewTrack(codec, streamer.DirectionSendonly),
	}
}

func (c *Client) GetMedias() []*streamer.Media {
	return []*streamer.Media{{
		Kind:      streamer.KindVideo,
		Direction: streamer.DirectionSendonly,
		Codecs:    []*streamer.Codec{c.track.Codec},
	}}
}

func (c *Client) GetTrack(media *streamer.Media, codec *streamer.Codec) *streamer.Track {
	return c.track
}

func (c *Client) Start() error {
	ct := c.res.Header.Get("Content-Type")

	if ct == "image/jpeg" {
		return c.startJPEG()
	}

	// added in go1.18
	if _, s, ok := strings.Cut(ct, "boundary="); ok {
		return c.startMJPEG(s)
	}

	return errors.New("wrong Content-Type: " + ct)
}

func (c *Client) Stop() error {
	c.closed = true
	return nil
}

func (c *Client) startJPEG() error {
	buf, err := io.ReadAll(c.res.Body)
	if err != nil {
		return err
	}

	packet := &rtp.Packet{Payload: buf}
	_ = c.track.WriteRTP(packet)

	req := c.res.Request

	for !c.closed {
		res, err := tcp.Do(req)
		if err != nil {
			return err
		}

		if res.StatusCode != http.StatusOK {
			return errors.New("wrong status: " + res.Status)
		}

		buf, err = io.ReadAll(res.Body)
		if err != nil {
			return err
		}

		packet = &rtp.Packet{Payload: buf}
		_ = c.track.WriteRTP(packet)
	}

	return nil
}

func (c *Client) startMJPEG(boundary string) error {
	boundary = "--" + boundary

	r := bufio.NewReader(c.res.Body)
	tp := textproto.NewReader(r)

	for !c.closed {
		s, err := tp.ReadLine()
		if err != nil {
			return err
		}
		if s != boundary {
			return errors.New("wrong boundary: " + s)
		}

		header, err := tp.ReadMIMEHeader()
		if err != nil {
			return err
		}

		s = header.Get("Content-Length")
		if s == "" {
			return errors.New("no content length")
		}

		size, err := strconv.Atoi(s)
		if err != nil {
			return err
		}

		buf := make([]byte, size)
		if _, err = io.ReadFull(r, buf); err != nil {
			return err
		}

		packet := &rtp.Packet{Payload: buf}
		_ = c.track.WriteRTP(packet)

		if _, err = r.Discard(2); err != nil {
			return err
		}
	}

	return nil
}
