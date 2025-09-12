package ipeye

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"

	mediacommonh264 "github.com/bluenviron/mediacommon/pkg/codecs/h264"
)

type Producer struct {
	core.Connection
	conn         *websocket.Conn
	videoTrackID uint32
	sps, pps     []byte
}

// Dial подключается к ipeye WebSocket с обязательным Origin
func Dial(source string) (core.Producer, error) {
	url, _ := strings.CutPrefix(source, "ipeye:")

	dialer := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
	}
	header := http.Header{}
	header.Set("Origin", "https://ipeye.ru")

	conn, _, err := dialer.Dial(url, header)
	if err != nil {
		return nil, err
	}

	prod := &Producer{
		Connection: core.Connection{
			ID:         core.NewID(),
			FormatName: "ipeye",
			Protocol:   "wss",
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

// probe ждёт init с avcC и извлекает SPS/PPS
func (p *Producer) probe() error {
	for {
		mType, b, err := p.conn.ReadMessage()
		if err != nil {
			return err
		}

		if mType == websocket.BinaryMessage {
			atoms, err := iso.DecodeAtoms(b)
			if err != nil {
				continue
			}

			var trackID uint32

			for _, atom := range atoms {
				switch atom := atom.(type) {
				case *iso.AtomTkhd:
					trackID = atom.TrackID
				case *iso.AtomVideo:
					if atom.Name == "avc1" {
						codec := h264.AVCCToCodec(atom.Config)
						sps, pps := parseSPSPPS(atom.Config)
						p.sps, p.pps = sps, pps

						p.Medias = append(p.Medias, &core.Media{
							Kind:      core.KindVideo,
							Direction: core.DirectionRecvonly,
							Codecs:    []*core.Codec{codec},
						})
						p.videoTrackID = trackID

						log.Printf("[ipeye] fMP4 video avc1 trackID=%d", trackID)
					}
				}
			}

			if len(p.Medias) > 0 {
				log.Printf("[ipeye] detected fMP4 init with %d medias", len(p.Medias))
				return nil
			}
		}
	}
}

// Start запускает основной цикл чтения фрагментов
func (p *Producer) Start() error {
	receivers := make(map[uint32]*core.Receiver)
	if p.videoTrackID != 0 {
		for _, receiver := range p.Receivers {
			if receiver.Codec.Kind() == core.KindVideo {
				receivers[p.videoTrackID] = receiver
			}
		}
	}

	// RTP packetizer
	h264Pay := &codecs.H264Payloader{}
	seq := rtp.NewRandomSequencer()
	h264pkt := rtp.NewPacketizer(1200, 96, 0, h264Pay, seq, 90000)

	var tsCounter uint32 = 0
	const frameDur = 90000 / 25 // фиксированный шаг для 25fps

	for {
		mType, b, err := p.conn.ReadMessage()
		if err != nil {
			log.Printf("[ipeye] read error: %v", err)
			return err
		}
		if mType != websocket.BinaryMessage {
			continue
		}

		atoms, err := iso.DecodeAtoms(b)
		if err != nil {
			continue
		}

		var trackID uint32
		var mdatData []byte

		for _, atom := range atoms {
			switch atom := atom.(type) {
			case *iso.AtomTfhd:
				trackID = atom.TrackID
			case *iso.AtomMdat:
				mdatData = atom.Data
			}
		}

		if recv := receivers[trackID]; recv != nil && len(mdatData) > 0 {
			var avcc mediacommonh264.AVCC
			if err := avcc.Unmarshal(mdatData); err != nil {
				log.Printf("[ipeye] avcc unmarshal error: %v", err)
				continue
			}

			for _, nalu := range avcc {
				typ := nalu[0] & 0x1F

				// Если IDR — добавляем SPS/PPS
				if typ == 5 {
					if len(p.sps) > 0 {
						for _, pkt := range h264pkt.Packetize(p.sps, tsCounter) {
							recv.WriteRTP(pkt)
						}
					}
					if len(p.pps) > 0 {
						for _, pkt := range h264pkt.Packetize(p.pps, tsCounter) {
							recv.WriteRTP(pkt)
						}
					}
				}

				for _, pkt := range h264pkt.Packetize(nalu, tsCounter) {
					recv.WriteRTP(pkt)
				}
			}
			tsCounter += frameDur
		}
	}
}

// parseSPSPPS парсит SPS/PPS из avcC
func parseSPSPPS(avcc []byte) (sps, pps []byte) {
	if len(avcc) < 7 {
		return
	}
	numSPS := int(avcc[5] & 0x1F)
	pos := 6
	for i := 0; i < numSPS && pos+2 <= len(avcc); i++ {
		size := int(avcc[pos])<<8 | int(avcc[pos+1])
		pos += 2
		if pos+size > len(avcc) {
			return
		}
		sps = avcc[pos : pos+size]
		pos += size
	}
	if pos >= len(avcc) {
		return
	}
	numPPS := int(avcc[pos])
	pos++
	for i := 0; i < numPPS && pos+2 <= len(avcc); i++ {
		size := int(avcc[pos])<<8 | int(avcc[pos+1])
		pos += 2
		if pos+size > len(avcc) {
			return
		}
		pps = avcc[pos : pos+size]
		pos += size
	}
	return
}
