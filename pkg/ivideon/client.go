package ivideon

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
)

type State byte

const (
	StateNone State = iota
	StateConn
	StateHandle
)

// Deprecated: should be rewritten to core.Connection
type Client struct {
	core.Listener

	ID string

	conn *websocket.Conn

	medias   []*core.Media
	receiver *core.Receiver

	msg *message
	t0  time.Time

	buffer chan []byte
	state  State
	mu     sync.Mutex

	recv int
}

func Dial(source string) (*Client, error) {
	id := strings.Replace(source[8:], "/", ":", 1)
	client := &Client{ID: id}
	if err := client.Dial(); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *Client) Dial() (err error) {
	resp, err := http.Get(
		"https://openapi-alpha.ivideon.com/cameras/" + c.ID +
			"/live_stream?op=GET&access_token=public&q=2&" +
			"video_codecs=h264&format=ws-fmp4",
	)

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var v liveResponse
	if err = json.Unmarshal(data, &v); err != nil {
		return err
	}

	if !v.Success {
		return fmt.Errorf("wrong response: %s", data)
	}

	c.conn, _, err = websocket.DefaultDialer.Dial(v.Result.URL, nil)
	if err != nil {
		return err
	}

	if err = c.getTracks(); err != nil {
		_ = c.conn.Close()
		return err
	}

	c.state = StateConn

	return nil
}

func (c *Client) Handle() error {
	// add delay to the stream for smooth playing (not a best solution)
	c.t0 = time.Now().Add(time.Second)

	c.mu.Lock()

	if c.state == StateConn {
		c.buffer = make(chan []byte, 5)
		c.state = StateHandle

		// processing stream in separate thread for lower delay between packets
		go c.worker(c.buffer)
	}

	c.mu.Unlock()

	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return err
	}

	if c.receiver != nil && c.receiver.ID == c.msg.Track {
		c.mu.Lock()
		if c.state == StateHandle {
			c.buffer <- data
			c.recv += len(data)
		}
		c.mu.Unlock()
	}

	// we have one unprocessed msg after getTracks
	for {
		_, data, err = c.conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg message
		if err = json.Unmarshal(data, &msg); err != nil {
			return err
		}

		switch msg.Type {
		case "stream-init":
			continue

		case "metadata":
			continue

		case "fragment":
			_, data, err = c.conn.ReadMessage()
			if err != nil {
				return err
			}

			if c.receiver != nil && c.receiver.ID == msg.Track {
				c.mu.Lock()
				if c.state == StateHandle {
					c.buffer <- data
					c.recv += len(data)
				}
				c.mu.Unlock()
			}

		default:
			return fmt.Errorf("wrong message type: %s", data)
		}
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch c.state {
	case StateNone:
		return nil
	case StateConn:
	case StateHandle:
		close(c.buffer)
	}

	c.state = StateNone

	return c.conn.Close()
}

func (c *Client) getTracks() error {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg message
		if err = json.Unmarshal(data, &msg); err != nil {
			return err
		}

		switch msg.Type {
		case "metadata":
			continue

		case "stream-init":
			s := msg.CodecString
			i := strings.IndexByte(s, '.')
			if i > 0 {
				s = s[:i]
			}

			switch s {
			case "avc1": // avc1.4d0029
				// skip multiple identical init
				if c.receiver != nil {
					continue
				}

				i = bytes.Index(msg.Data, []byte("avcC")) - 4
				if i < 0 {
					return fmt.Errorf("ivideon: wrong AVC: %s", msg.Data)
				}

				avccLen := binary.BigEndian.Uint32(msg.Data[i:])
				data = msg.Data[i+8 : i+int(avccLen)]

				codec := h264.ConfigToCodec(data)

				media := &core.Media{
					Kind:      core.KindVideo,
					Direction: core.DirectionRecvonly,
					Codecs:    []*core.Codec{codec},
				}
				c.medias = append(c.medias, media)

				c.receiver = core.NewReceiver(media, codec)
				c.receiver.ID = msg.TrackID

			case "mp4a": // mp4a.40.2
			}

		case "fragment":
			c.msg = &msg
			return nil

		default:
			return fmt.Errorf("wrong message type: %s", data)
		}
	}
}

func (c *Client) worker(buffer chan []byte) {
	for data := range buffer {
		atoms, err := iso.DecodeAtoms(data)
		if err != nil {
			continue
		}

		var trun *iso.Atom
		var ts uint32

		for _, atom := range atoms {
			switch atom.Name {
			case iso.MoofTrafTrun:
				trun = atom
			case iso.MoofTrafTfdt:
				ts = uint32(atom.DecodeTime)
			case iso.Mdat:
				data = atom.Data
			}
		}

		if trun == nil || trun.SamplesDuration == nil || trun.SamplesSize == nil {
			continue
		}

		for i := 0; i < len(trun.SamplesDuration); i++ {
			duration := trun.SamplesDuration[i]
			size := trun.SamplesSize[i]

			// synchronize framerate for WebRTC and MSE
			d := time.Duration(ts)*time.Millisecond - time.Since(c.t0)
			if d < 0 {
				d = time.Duration(duration) * time.Millisecond / 2
			}
			time.Sleep(d)

			// can be SPS, PPS and IFrame in one packet
			packet := &rtp.Packet{
				// ivideon clockrate=1000, RTP clockrate=90000
				Header:  rtp.Header{Timestamp: ts * 90},
				Payload: data[:size],
			}
			c.receiver.WriteRTP(packet)

			data = data[size:]
			ts += duration
		}
	}
}

type liveResponse struct {
	Result struct {
		URL string `json:"url"`
	} `json:"result"`
	Success bool `json:"success"`
}

type message struct {
	Type string `json:"type"`

	CodecString string `json:"codec_string"`
	Data        []byte `json:"data"`
	TrackID     byte   `json:"track_id"`

	Track      byte    `json:"track"`
	StartTime  float32 `json:"start_time"`
	Duration   float32 `json:"duration"`
	IsKey      bool    `json:"is_key"`
	DataOffset uint32  `json:"data_offset"`
}
