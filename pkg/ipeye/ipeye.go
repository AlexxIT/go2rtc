package ipeye

import (
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/iso"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
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
						p.clockRate = codec.ClockRate
					}
				}
			}

			if len(p.Medias) > 0 {
				return nil
			}
		}
	}
}

// Start runs the main fragment reading loop
func (p *Producer) Start() error {

	receivers := make(map[uint32]*core.Receiver)
	if p.videoTrackID != 0 {
		for _, receiver := range p.Receivers {
			if receiver.Codec.Kind() == core.KindVideo {
				receivers[p.videoTrackID] = receiver
			}
		}
	}

	// RTP counters
	rtpStart := rand.Uint32()
	seq := uint16(rand.Uint32())
	var dts uint64
	var defaultDur uint32
	var initialized bool

	const wrapPeriod = uint64(1) << 32 // RTP TS wrap (mod 2^32)

	for {
		mType, b, err := p.conn.ReadMessage()
		if err != nil {
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

		// RTP TS for this sample
		ts := rtpStart + uint32((dts*uint64(p.clockRate)/90000)%wrapPeriod)

		// Send one RTP packet with payload in AVCC
		seq++
		recv.Input(&rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: seq,
				Timestamp:      ts,
				SSRC:           1,
			},
			Payload: mdatData,
		})

		// Choose duration
		dur := defaultDur
		if len(samplesDur) > 0 && samplesDur[0] != 0 {
			dur = samplesDur[0]
		}
		if dur == 0 {
			dur = p.clockRate / 25 // fallback
		}

		// Increment DTS
		dts += uint64(dur)
		if dts >= wrapPeriod {
			dts %= wrapPeriod
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
