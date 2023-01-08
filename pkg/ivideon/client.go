package ivideon

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/fmp4/fmp4io"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	streamer.Element

	ID string

	conn   *websocket.Conn
	medias []*streamer.Media
	tracks map[byte]*streamer.Track

	closed bool

	msg *message
	t0  time.Time

	buffer chan []byte
}

func NewClient(id string) *Client {
	return &Client{ID: id}
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

	return nil
}

func (c *Client) Handle() error {
	c.buffer = make(chan []byte, 5)
	// add delay to the stream for smooth playing (not a best solution)
	c.t0 = time.Now().Add(time.Second)

	// processing stream in separate thread for lower delay between packets
	go c.worker()

	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return err
	}

	track := c.tracks[c.msg.Track]
	if track != nil {
		c.buffer <- data
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

		case "fragment":
			_, data, err = c.conn.ReadMessage()
			if err != nil {
				return err
			}

			track = c.tracks[msg.Track]
			if track != nil {
				c.buffer <- data
			}

		default:
			return fmt.Errorf("wrong message type: %s", data)
		}
	}
}

func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	if c.buffer != nil {
		close(c.buffer)
	}
	c.closed = true
	return c.conn.Close()
}

func (c *Client) getTracks() error {
	c.tracks = map[byte]*streamer.Track{}

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
		case "stream-init":
			s := msg.CodecString
			i := strings.IndexByte(s, '.')
			if i > 0 {
				s = s[:i]
			}

			switch s {
			case "avc1": // avc1.4d0029
				// skip multiple identical init
				if c.tracks[msg.TrackID] != nil {
					continue
				}

				codec := &streamer.Codec{
					Name:        streamer.CodecH264,
					ClockRate:   90000,
					FmtpLine:    "profile-level-id=" + msg.CodecString[i+1:],
					PayloadType: streamer.PayloadTypeRAW,
				}

				i = bytes.Index(msg.Data, []byte("avcC")) - 4
				if i < 0 {
					return fmt.Errorf("wrong AVC: %s", msg.Data)
				}

				avccLen := binary.BigEndian.Uint32(msg.Data[i:])
				data = msg.Data[i+8 : i+int(avccLen)]

				record := h264parser.AVCDecoderConfRecord{}
				if _, err = record.Unmarshal(data); err != nil {
					return err
				}

				codec.FmtpLine += ";sprop-parameter-sets=" +
					base64.StdEncoding.EncodeToString(record.SPS[0]) + "," +
					base64.StdEncoding.EncodeToString(record.PPS[0])

				media := &streamer.Media{
					Kind:      streamer.KindVideo,
					Direction: streamer.DirectionSendonly,
					Codecs:    []*streamer.Codec{codec},
				}
				c.medias = append(c.medias, media)

				track := streamer.NewTrack(codec, streamer.DirectionSendonly)
				c.tracks[msg.TrackID] = track

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

func (c *Client) worker() {
	var track *streamer.Track
	for _, track = range c.tracks {
		break
	}

	for data := range c.buffer {
		moof := &fmp4io.MovieFrag{}
		if _, err := moof.Unmarshal(data, 0); err != nil {
			continue
		}

		moofLen := binary.BigEndian.Uint32(data)
		_ = moofLen

		mdat := moof.Unknowns[0]
		if mdat.Tag() != fmp4io.MDAT {
			continue
		}
		i, _ := mdat.Pos() // offset, size
		data = data[i+8:]

		traf := moof.Tracks[0]
		ts := uint32(traf.DecodeTime.Time)

		//println("!!!", (time.Duration(ts) * time.Millisecond).String(), time.Since(c.t0).String())

		for _, entry := range traf.Run.Entries {
			// synchronize framerate for WebRTC and MSE
			d := time.Duration(ts)*time.Millisecond - time.Since(c.t0)
			if d < 0 {
				d = time.Duration(entry.Duration) * time.Millisecond / 2
			}
			time.Sleep(d)

			// can be SPS, PPS and IFrame in one packet
			packet := &rtp.Packet{
				// ivideon clockrate=1000, RTP clockrate=90000
				Header:  rtp.Header{Timestamp: ts * 90},
				Payload: data[:entry.Size],
			}
			_ = track.WriteRTP(packet)

			data = data[entry.Size:]
			ts += entry.Duration
		}

		if len(data) != 0 {
			continue
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
