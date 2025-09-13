package ipeye

import (
	"bytes"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

type Producer struct {
	core.Connection
	conn *websocket.Conn

	videoTrackID   uint32
	clockRate      uint32
	sps, pps       []byte
	baseSet        bool
	baseDecodeTime uint64
}

// Dial connects to ipeye WebSocket with required Origin
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

// probe waits for init with avcC and extracts SPS/PPS
func (p *Producer) probe() error {
	log := app.GetLogger("ipeye")

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

			var trackID, timeScale uint32

			for _, atom := range atoms {
				switch atom := atom.(type) {
				case *iso.AtomTkhd:
					trackID = atom.TrackID
				case *iso.AtomMdhd:
					timeScale = atom.TimeScale
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
						p.clockRate = codec.ClockRate

						log.Info().
							Uint32("trackID", trackID).
							Uint32("timeScale", timeScale).
							Msg("fMP4 video detected")
					}
				}
			}

			if len(p.Medias) > 0 {
				log.Info().Int("medias", len(p.Medias)).Msg("fMP4 init complete")
				return nil
			}
		}
	}
}

// Start runs the main fragment reading loop
func (p *Producer) Start() error {
	log := app.GetLogger("ipeye")

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
	h264pkt := rtp.NewPacketizer(1200, 96, 0, h264Pay, seq, p.clockRate)

	// global counters
	rtpStart := rand.Uint32()
	var dts uint64
	var defaultDur uint32
	var initialized bool

	const wrapPeriod = uint64(1) << 32 // RTP TS wraps (mod 2^32)

	for {
		mType, b, err := p.conn.ReadMessage()
		if err != nil {
			log.Error().Err(err).Msg("read error")
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
		var samplesDur []uint32

		for _, atom := range atoms {
			switch atom := atom.(type) {
			case *iso.AtomTfhd:
				trackID = atom.TrackID
				defaultDur = atom.SampleDuration

			case *iso.AtomTfdt:
				if !initialized {
					log.Info().
						Uint64("decodeTime", atom.DecodeTime).
						Uint32("rtpStart", rtpStart).
						Msg("stream initialized")
					dts = atom.DecodeTime
					initialized = true
				}

			case *iso.AtomTrun:
				samplesDur = atom.SamplesDuration

			case *iso.AtomMdat:
				mdatData = atom.Data
			}
		}

		recv := receivers[trackID]
		if recv == nil || len(mdatData) == 0 {
			continue
		}

		// convert AVCC -> AnnexB
		annexbData := annexb.DecodeAVCC(mdatData, true)
		nalus := bytes.Split(annexbData, []byte{0, 0, 0, 1})

		// iterate over NALUs
		for i, nalu := range nalus {
			if len(nalu) == 0 {
				continue
			}
			typ := nalu[0] & 0x1F

			// RTP TS for this sample (mod 2^32)
			ts := rtpStart + uint32((dts*uint64(p.clockRate)/90000)%wrapPeriod)

			// SPS/PPS before IDR
			if typ == h264.NALUTypeIFrame {
				if len(p.sps) > 0 {
					for _, pkt := range h264pkt.Packetize(p.sps, ts) {
						pkt.Timestamp = ts
						recv.Input(pkt)
						log.Debug().
							Uint32("ts", pkt.Timestamp).
							Uint16("seq", pkt.SequenceNumber).
							Int("size", len(pkt.Payload)).
							Str("type", "SPS").
							Msg("RTP packet")
					}
				}
				if len(p.pps) > 0 {
					for _, pkt := range h264pkt.Packetize(p.pps, ts) {
						pkt.Timestamp = ts
						recv.Input(pkt)
						log.Debug().
							Uint32("ts", pkt.Timestamp).
							Uint16("seq", pkt.SequenceNumber).
							Int("size", len(pkt.Payload)).
							Str("type", "PPS").
							Msg("RTP packet")
					}
				}
			}

			// actual frame
			for _, pkt := range h264pkt.Packetize(nalu, ts) {
				pkt.Timestamp = ts
				recv.Input(pkt)
				log.Debug().
					Uint32("ts", pkt.Timestamp).
					Uint16("seq", pkt.SequenceNumber).
					Int("size", len(pkt.Payload)).
					Int("nalType", int(typ)).
					Msg("RTP packet")
			}

			// duration selection
			dur := defaultDur
			if i < len(samplesDur) && samplesDur[i] != 0 {
				dur = samplesDur[i]
			}
			if dur == 0 {
				dur = p.clockRate / 25 // fallback
			}

			log.Trace().
				Int("sample", i).
				Uint32("dur", dur).
				Uint64("dts", dts).
				Uint32("rtpTS", ts).
				Int("nalType", int(typ)).
				Int("size", len(nalu)).
				Msg("sample processed")

			// increment DTS
			dts += uint64(dur)
			if dts >= wrapPeriod {
				dts %= wrapPeriod
			}
		}
	}
}

// parseSPSPPS extracts SPS/PPS from avcC
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
