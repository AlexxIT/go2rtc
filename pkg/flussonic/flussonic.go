package flussonic

import (
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
)

type Producer struct {
	core.Connection
	conn *websocket.Conn

	videoTrackID, audioTrackID     uint32
	videoTimeScale, audioTimeScale float32
}

func Dial(source string) (core.Producer, error) {
	url, _ := strings.CutPrefix(source, "flussonic:")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "flussonic",
			Protocol:   core.Before(url, ":"), // wss
			RemoteAddr: conn.RemoteAddr().String(),
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

func (p *Producer) probe() error {
	var init struct {
		//Metadata struct {
		//	Tracks []struct {
		//		Width   int    `json:"width,omitempty"`
		//		Height  int    `json:"height,omitempty"`
		//		Fps     int    `json:"fps,omitempty"`
		//		Content string `json:"content"`
		//		TrackId string `json:"trackId"`
		//		Bitrate int    `json:"bitrate"`
		//	} `json:"tracks"`
		//} `json:"metadata"`
		Tracks []struct {
			Content string `json:"content"`
			Id      uint32 `json:"id"`
			Payload []byte `json:"payload"`
		} `json:"tracks"`
		//Type string `json:"type"`
	}

	if err := p.conn.ReadJSON(&init); err != nil {
		return err
	}

	var timeScale uint32

	for _, track := range init.Tracks {
		atoms, _ := iso.DecodeAtoms(track.Payload)
		for _, atom := range atoms {
			switch atom := atom.(type) {
			case *iso.AtomMdhd:
				timeScale = atom.TimeScale
			case *iso.AtomVideo:
				switch atom.Name {
				case "avc1":
					codec := h264.AVCCToCodec(atom.Config)
					p.Medias = append(p.Medias, &core.Media{
						Kind:      core.KindVideo,
						Direction: core.DirectionRecvonly,
						Codecs:    []*core.Codec{codec},
					})
					p.videoTrackID = track.Id
					p.videoTimeScale = float32(codec.ClockRate) / float32(timeScale)
				}
			case *iso.AtomAudio:
				switch atom.Name {
				case "mp4a":
					codec := aac.ConfigToCodec(atom.Config)
					p.Medias = append(p.Medias, &core.Media{
						Kind:      core.KindAudio,
						Direction: core.DirectionRecvonly,
						Codecs:    []*core.Codec{codec},
					})
					p.audioTrackID = track.Id
					p.audioTimeScale = float32(codec.ClockRate) / float32(timeScale)
				}
			}
		}
	}

	return nil
}

func (p *Producer) Start() error {
	if err := p.conn.WriteMessage(websocket.TextMessage, []byte("resume")); err != nil {
		return err
	}

	receivers := make(map[uint32]*core.Receiver)
	timeScales := make(map[uint32]float32)

	for _, receiver := range p.Receivers {
		switch receiver.Codec.Kind() {
		case core.KindVideo:
			receivers[p.videoTrackID] = receiver
			timeScales[p.videoTrackID] = p.videoTimeScale
		case core.KindAudio:
			receivers[p.audioTrackID] = receiver
			timeScales[p.audioTrackID] = p.audioTimeScale
		}
	}

	ch := make(chan []byte, 10)
	defer close(ch)

	go func() {
		for b := range ch {
			atoms, err := iso.DecodeAtoms(b)
			if err != nil {
				continue
			}

			var trackID uint32
			var decodeTime uint64

			for _, atom := range atoms {
				switch atom := atom.(type) {
				case *iso.AtomTfhd:
					trackID = atom.TrackID
				case *iso.AtomTfdt:
					decodeTime = atom.DecodeTime
				case *iso.AtomMdat:
					b = atom.Data
				}
			}

			if recv := receivers[trackID]; recv != nil {
				timestamp := uint32(float32(decodeTime) * timeScales[trackID])
				packet := &rtp.Packet{
					Header:  rtp.Header{Timestamp: timestamp},
					Payload: b,
				}
				recv.WriteRTP(packet)
			}
		}
	}()

	for {
		mType, b, err := p.conn.ReadMessage()
		if err != nil {
			return err
		}
		if mType == websocket.BinaryMessage {
			p.Recv += len(b)
			ch <- b
		}
	}
}
