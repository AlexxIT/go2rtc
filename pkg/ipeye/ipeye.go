package ipeye

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
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

// первый пакет содержит строку кодеков ("avc1.42001F,mp4a.40.2")
func (p *Producer) probe() error {
	_, b, err := p.conn.ReadMessage()
	if err != nil {
		return err
	}
	if len(b) == 0 || b[0] != 6 {
		return errors.New("ipeye: invalid first packet (codec info not found)")
	}

	codecStr := string(b[1:])
	log.Printf("[ipeye] codecs string: %s", codecStr)

	if strings.Contains(codecStr, "avc1") {
		// создаём provisional H264 codec без SPS/PPS
		codec := &core.Codec{
			Name:        core.CodecH264,
			ClockRate:   90000,
			FmtpLine:    "packetization-mode=1",
			PayloadType: core.PayloadTypeRAW,
		}
		p.Medias = append(p.Medias, &core.Media{
			Kind:      core.KindVideo,
			Direction: core.DirectionRecvonly,
			Codecs:    []*core.Codec{codec},
		})
		log.Printf("[ipeye] provisional H264 codec created")
	}
	return nil
}

func (p *Producer) Start() error {
	receivers := make(map[uint32]*core.Receiver)
	for _, receiver := range p.Receivers {
		if receiver.Codec.Kind() == core.KindVideo {
			receivers[0] = receiver // trackID узнаем позже
		}
	}

	sequencer := rtp.NewRandomSequencer()
	payloader := &h264.Payloader{IsAVC: false}

	log.Printf("[ipeye] start streaming")

	var trackDetected bool

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
			log.Printf("[ipeye] iso.DecodeAtoms error: %v", err)
			continue
		}

		var trackID uint32
		var decodeTime uint64
		var data []byte

		for _, atom := range atoms {
			switch atom := atom.(type) {
			case *iso.AtomTfhd:
				trackID = atom.TrackID
				if !trackDetected {
					p.videoTrackID = trackID
					if recv0, ok := receivers[0]; ok {
						receivers = make(map[uint32]*core.Receiver)
						receivers[trackID] = recv0
					}
					trackDetected = true
					log.Printf("[ipeye] detected video trackID=%d", trackID)
				}
			case *iso.AtomTfdt:
				decodeTime = atom.DecodeTime
			case *iso.AtomMdat:
				data = atom.Data
			}
		}

		if recv := receivers[trackID]; recv != nil && len(data) > 0 {
			annex := annexb.DecodeAVCC(data, true)
			if annex == nil {
				continue
			}

			// если ещё нет SPS/PPS — пробуем достать
			if p.sps == nil || p.pps == nil {
				sps, pps := extractSPSPPS(annex)
				if sps != nil && pps != nil {
					p.sps, p.pps = sps, pps
					codec := h264.ConfigToCodec(h264.EncodeConfig(sps, pps))
					p.Medias[0].Codecs = []*core.Codec{codec}
					log.Printf("[ipeye] Codec updated with SPS/PPS: %s", codec.FmtpLine)
				}
			}

			// timestamp в 90 кГц
			ts := uint32(decodeTime * 90)

			// если это ключевой кадр — добавляем SPS/PPS в поток
			if h264.IsKeyframe(annex) && p.sps != nil && p.pps != nil {
				ps := h264.JoinNALU(p.sps, p.pps)
				annex = append(ps, annex...)
				log.Printf("[ipeye] prepended SPS/PPS to keyframe")
			}

			payloads := payloader.Payload(1400, annex)
			if len(payloads) == 0 {
				continue
			}

			last := len(payloads) - 1
			for i, pl := range payloads {
				pkt := &rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						PayloadType:    recv.Codec.PayloadType,
						SequenceNumber: sequencer.NextSequenceNumber(),
						Timestamp:      ts,
						Marker:         i == last,
					},
					Payload: pl,
				}
				recv.WriteRTP(pkt)
			}

			log.Printf("[ipeye] sent frame track=%d ts=%d parts=%d", trackID, ts, len(payloads))
		}
	}
}

// extractSPSPPS ищет SPS/PPS в AnnexB-потоке
func extractSPSPPS(b []byte) (sps, pps []byte) {
	const startCode = "\x00\x00\x00\x01"
	for {
		i := bytes.Index(b, []byte(startCode))
		if i < 0 || i+4 >= len(b) {
			break
		}
		b = b[i+4:]

		ntype := h264.NALUType(b)
		size := nextNALUSize(b)

		switch ntype {
		case h264.NALUTypeSPS:
			if sps == nil {
				sps = b[:size]
			}
		case h264.NALUTypePPS:
			if pps == nil {
				pps = b[:size]
			}
		}

		if sps != nil && pps != nil {
			return
		}
		if size <= 0 || size >= len(b) {
			break
		}
		b = b[size:]
	}
	return
}

func nextNALUSize(b []byte) int {
	const startCode = "\x00\x00\x00\x01"
	i := bytes.Index(b, []byte(startCode))
	if i < 0 {
		return len(b)
	}
	return i
}
