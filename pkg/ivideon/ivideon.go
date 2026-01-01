package ivideon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/gorilla/websocket"
)

type Producer struct {
	core.Connection
	conn *websocket.Conn

	buf []byte

	dem *mp4.Demuxer
}

func Dial(source string) (core.Producer, error) {
	id := strings.Replace(source[8:], "/", ":", 1)

	url, err := GetLiveStream(id)
	if err != nil {
		return nil, err
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "ivideon",
			Protocol:   core.Before(url, ":"), // wss
			RemoteAddr: conn.RemoteAddr().String(),
			Source:     source,
			URL:        url,
			Transport:  conn,
		},
		conn: conn,
	}

	if err = prod.probe(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return prod, nil
}

func GetLiveStream(id string) (string, error) {
	// &video_codecs=h264,h265&audio_codecs=aac,mp3,pcma,pcmu,none
	resp, err := http.Get(
		"https://openapi-alpha.ivideon.com/cameras/" + id +
			"/live_stream?op=GET&access_token=public&q=2&video_codecs=h264&format=ws-fmp4",
	)
	if err != nil {
		return "", err
	}

	var v struct {
		Message string `json:"message"`
		Result  struct {
			URL string `json:"url"`
		} `json:"result"`
		Success bool `json:"success"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}

	if !v.Success {
		return "", fmt.Errorf("ivideon: can't get live_stream: " + v.Message)
	}

	return v.Result.URL, nil
}

func (p *Producer) Start() error {
	receivers := make(map[uint32]*core.Receiver)
	for _, receiver := range p.Receivers {
		trackID := p.dem.GetTrackID(receiver.Codec)
		receivers[trackID] = receiver
	}

	ch := make(chan []byte, 10)
	defer close(ch)

	ch <- p.buf

	go func() {
		// add delay to the stream for smooth playing (not a best solution)
		t0 := time.Now()

		for data := range ch {
			trackID, packets := p.dem.Demux(data)
			if receiver := receivers[trackID]; receiver != nil {
				clockRate := time.Duration(receiver.Codec.ClockRate)
				for _, packet := range packets {
					// synchronize framerate for WebRTC and MSE
					ts := time.Second * time.Duration(packet.Timestamp) / clockRate
					d := ts - time.Since(t0)
					if d < 0 {
						d = 10 * time.Millisecond
					}
					time.Sleep(d)

					receiver.WriteRTP(packet)
				}
			}
		}
	}()

	for {
		var msg message
		if err := p.conn.ReadJSON(&msg); err != nil {
			return err
		}

		switch msg.Type {
		case "stream-init", "metadata":
			continue

		case "fragment":
			_, b, err := p.conn.ReadMessage()
			if err != nil {
				return err
			}

			p.Recv += len(b)
			ch <- b

		default:
			return errors.New("ivideon: wrong message type: " + msg.Type)
		}
	}
}

func (p *Producer) probe() (err error) {
	p.dem = &mp4.Demuxer{}

	for {
		var msg message
		if err = p.conn.ReadJSON(&msg); err != nil {
			return err
		}

		switch msg.Type {
		case "metadata":
			continue

		case "stream-init":
			// it's difficult to maintain audio
			if strings.HasPrefix(msg.CodecString, "avc1") {
				medias := p.dem.Probe(msg.Data)
				p.Medias = append(p.Medias, medias...)
			}

		case "fragment":
			_, p.buf, err = p.conn.ReadMessage()
			return

		default:
			return errors.New("ivideon: wrong message type: " + msg.Type)
		}
	}
}

type message struct {
	Type        string `json:"type"`
	CodecString string `json:"codec_string"`
	Data        []byte `json:"data"`
	//TrackID     byte    `json:"track_id"`
	//Track       byte    `json:"track"`
	//StartTime   float32 `json:"start_time"`
	//Duration    float32 `json:"duration"`
	//IsKey       bool    `json:"is_key"`
	//DataOffset  uint32  `json:"data_offset"`
}
