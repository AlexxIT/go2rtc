// Package bubble, because:
// Request URL: /bubble/live?ch=0&stream=0
// Response Conten-Type: video/bubble
// https://github.com/Lynch234ok/lynch-git/blob/master/app_rebulid/src/bubble.c
package bubble

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/tcp"
	"github.com/pion/rtp"
)

// Deprecated: should be rewritten to core.Connection
type Client struct {
	core.Listener

	url  string
	conn net.Conn

	videoCodec string
	channel    int
	stream     int

	r *bufio.Reader

	medias    []*core.Media
	receivers []*core.Receiver

	videoTrack *core.Receiver
	audioTrack *core.Receiver

	recv int
}

func Dial(rawURL string) (*Client, error) {
	client := &Client{url: rawURL}
	if err := client.Dial(); err != nil {
		return nil, err
	}
	return client, nil
}

const (
	SyncByte    = 0xAA
	PacketAuth  = 0x00
	PacketMedia = 0x01
	PacketStart = 0x0A
)

const Timeout = time.Second * 5

func (c *Client) Dial() (err error) {
	u, err := url.Parse(c.url)
	if err != nil {
		return
	}

	if c.conn, err = net.DialTimeout("tcp", u.Host, Timeout); err != nil {
		return
	}

	if err = c.conn.SetDeadline(time.Now().Add(Timeout)); err != nil {
		return
	}

	req := &tcp.Request{Method: "GET", URL: &url.URL{Path: u.Path, RawQuery: u.RawQuery}, Proto: "HTTP/1.1"}
	if err = req.Write(c.conn); err != nil {
		return
	}

	c.r = bufio.NewReader(c.conn)
	res, err := tcp.ReadResponse(c.r)
	if err != nil {
		return
	}

	if res.StatusCode != http.StatusOK {
		return errors.New("wrong response: " + res.Status)
	}

	// 1. Read 1024 bytes with XML, some cameras returns exact 1024, but some - 923
	xml := make([]byte, 1024)
	if _, err = c.r.Read(xml); err != nil {
		return
	}

	// 2. Write size uint32 + unknown 4b + user 20b + pass 20b
	b := make([]byte, 48)
	binary.BigEndian.PutUint32(b, 44)

	if u.User != nil {
		copy(b[8:], u.User.Username())
		pass, _ := u.User.Password()
		copy(b[28:], pass)
	} else {
		copy(b[8:], "admin")
	}

	if err = c.Write(PacketAuth, 0x0E16C271, b); err != nil {
		return
	}

	// 3. Read response
	cmd, b, err := c.Read()
	if err != nil {
		return
	}

	if cmd != PacketAuth || len(b) != 44 || b[4] != 3 || b[8] != 1 {
		return errors.New("wrong auth response")
	}

	// 4. Parse XML (from 1)
	query := u.Query()

	stream := query.Get("stream")
	if stream != "" {
		c.stream = core.Atoi(stream)
	} else {
		stream = "0"
	}

	// <bubble version="1.0" vin="1"><vin0 stream="2">
	// <stream0 name="720p.264" size="2304x1296" x1="yes" x2="yes" x4="yes" />
	// <stream1 name="360p.265" size="640x360" x1="yes" x2="yes" x4="yes" />
	// <vin0>
	// </bubble>
	re := regexp.MustCompile("<stream" + stream + " [^>]+")
	stream = re.FindString(string(xml))
	if strings.Contains(stream, ".265") {
		c.videoCodec = core.CodecH265
	} else {
		c.videoCodec = core.CodecH264
	}

	if ch := query.Get("ch"); ch != "" {
		c.channel = core.Atoi(ch)
	}

	return
}

func (c *Client) Write(command byte, timestamp uint32, payload []byte) error {
	if err := c.conn.SetWriteDeadline(time.Now().Add(Timeout)); err != nil {
		return err
	}

	// 0xAA + size uint32 + cmd byte + ts uint32 + payload
	b := make([]byte, 14+len(payload))
	b[0] = SyncByte
	binary.BigEndian.PutUint32(b[1:], uint32(5+len(payload)))
	b[5] = command
	binary.BigEndian.PutUint32(b[6:], timestamp)
	copy(b[10:], payload)

	_, err := c.conn.Write(b)
	return err
}

func (c *Client) Read() (byte, []byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(Timeout)); err != nil {
		return 0, nil, err
	}

	// 0xAA + size uint32 + cmd byte + ts uint32 + payload
	b := make([]byte, 10)
	if _, err := io.ReadFull(c.r, b); err != nil {
		return 0, nil, err
	}

	if b[0] != SyncByte {
		return 0, nil, errors.New("wrong start byte")
	}

	size := binary.BigEndian.Uint32(b[1:])
	payload := make([]byte, size-1-4)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return 0, nil, err
	}

	//timestamp := binary.BigEndian.Uint32(b[6:]) // in ms

	return b[5], payload, nil
}

func (c *Client) Play() error {
	// yeah, there's no mistake about the little endian
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, uint32(c.channel))
	binary.LittleEndian.PutUint32(b[4:], uint32(c.stream))
	binary.LittleEndian.PutUint32(b[8:], 1) // opened
	return c.Write(PacketStart, 0x0E16C2DF, b)
}

func (c *Client) Handle() error {
	var audioTS uint32

	for {
		cmd, b, err := c.Read()
		if err != nil {
			return err
		}

		c.recv += len(b)

		if cmd != PacketMedia {
			continue
		}

		// size uint32 + type 1b + channel 1b
		// type = 1 for keyframe, 2 for other frame, 0 for audio

		if b[4] > 0 {
			if c.videoTrack == nil {
				continue
			}

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Timestamp: core.Now90000(),
				},
				Payload: annexb.EncodeToAVCC(b[6:], false),
			}
			c.videoTrack.WriteRTP(pkt)
		} else {
			if c.audioTrack == nil {
				continue
			}

			//binary.LittleEndian.Uint32(b[6:])          // entries (always 1)
			//size := binary.LittleEndian.Uint32(b[10:]) // size
			//mk := binary.LittleEndian.Uint64(b[14:]) // pts (uint64_t)
			//binary.LittleEndian.Uint32(b[22:])         // gtime (time_t)
			//name := b[26:34] // g711
			//rate := binary.LittleEndian.Uint32(b[34:])  // sample rate
			//width := binary.LittleEndian.Uint32(b[38:]) // samplewidth

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:   2,
					Marker:    true,
					Timestamp: audioTS,
				},
				Payload: b[6+36:],
			}
			audioTS += uint32(len(pkt.Payload))
			c.audioTrack.WriteRTP(pkt)
		}
	}
}

func (c *Client) Close() error {
	return c.conn.Close()
}
