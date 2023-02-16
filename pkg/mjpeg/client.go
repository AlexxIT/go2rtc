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
	"sync/atomic"
	"time"
)

type Client struct {
	streamer.Element

	UserAgent  string
	RemoteAddr string

	closed bool
	res    *http.Response

	medias []*streamer.Media
	track  *streamer.Track
	recv   uint32
}

func NewClient(res *http.Response) *Client {
	return &Client{res: res}
}

func (c *Client) startJPEG() error {
	buf, err := io.ReadAll(c.res.Body)
	if err != nil {
		return err
	}

	packet := &rtp.Packet{Header: rtp.Header{Timestamp: now()}, Payload: buf}
	_ = c.track.WriteRTP(packet)

	atomic.AddUint32(&c.recv, uint32(len(buf)))

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

		packet = &rtp.Packet{Header: rtp.Header{Timestamp: now()}, Payload: buf}
		_ = c.track.WriteRTP(packet)

		atomic.AddUint32(&c.recv, uint32(len(buf)))
	}

	return nil
}

func (c *Client) startMJPEG(boundary string) error {
	// some cameras add prefix to boundary header:
	// https://github.com/TheTimeWalker/wallpanel-android
	if !strings.HasPrefix(boundary, "--") {
		boundary = "--" + boundary
	}

	r := bufio.NewReader(c.res.Body)
	tp := textproto.NewReader(r)

	for !c.closed {
		s, err := tp.ReadLine()
		if err != nil {
			return err
		}
		if !strings.HasPrefix(s, boundary) {
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

		packet := &rtp.Packet{Header: rtp.Header{Timestamp: now()}, Payload: buf}
		_ = c.track.WriteRTP(packet)

		atomic.AddUint32(&c.recv, uint32(len(buf)))

		if _, err = r.Discard(2); err != nil {
			return err
		}
	}

	return nil
}

func now() uint32 {
	return uint32(time.Now().UnixMilli() * 90)
}
