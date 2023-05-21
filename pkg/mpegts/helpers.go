package mpegts

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/pion/rtp"
	"time"
)

const (
	PacketSize = 188
	SyncByte   = 0x47
)

const (
	StreamTypePrivate  = 0x06 // PCMU or PCMA or FLAC from FFmpeg
	StreamTypeAAC      = 0x0F
	StreamTypeH264     = 0x1B
	StreamTypeH265     = 0x24
	StreamTypePCMATapo = 0x90
)

type Packet struct {
	StreamType byte
	PTS        time.Duration
	DTS        time.Duration
	Payload    []byte
}

// PES - Packetized Elementary Stream
type PES struct {
	StreamType byte
	StreamID   byte
	Payload    []byte
	Mode       byte
	Size       int

	Sequence  uint16
	Timestamp uint32

	decodeStream func([]byte) ([]byte, int)
}

const (
	ModeUnknown = iota
	ModeSize
	ModeStream
)

// parse Optional PES header
const minHeaderSize = 3

func (p *PES) SetBuffer(size uint16, b []byte) {
	if size == 0 {
		optSize := b[2] // optional fields
		b = b[minHeaderSize+optSize:]

		switch p.StreamType {
		case StreamTypeH264:
			p.Mode = ModeStream
			p.decodeStream = h264.DecodeStream
		case StreamTypeH265:
			p.Mode = ModeStream
			p.decodeStream = h265.DecodeStream
		default:
			println("WARNING: mpegts: unknown zero-size stream")
		}
	} else {
		p.Mode = ModeSize
		p.Size = int(size)
	}

	p.Payload = make([]byte, 0, size)
	p.Payload = append(p.Payload, b...)
}

func (p *PES) AppendBuffer(b []byte) {
	p.Payload = append(p.Payload, b...)
}

func (p *PES) GetPacket() (pkt *rtp.Packet) {
	switch p.Mode {
	case ModeSize:
		left := p.Size - len(p.Payload)
		if left > 0 {
			return
		}

		if left < 0 {
			println("WARNING: mpegts: buffer overflow")
			p.Payload = nil
			return
		}

		// fist byte also flags
		flags := p.Payload[1]
		optSize := p.Payload[2] // optional fields

		payload := p.Payload[minHeaderSize+optSize:]

		switch p.StreamType {
		case StreamTypeH264, StreamTypeH265:
			var ts uint32

			const hasPTS = 0b1000_0000
			if flags&hasPTS != 0 {
				ts = ParseTime(p.Payload[minHeaderSize:])
			}

			pkt = &rtp.Packet{
				Header: rtp.Header{
					PayloadType: p.StreamType,
					Timestamp:   ts,
				},
				Payload: h264.AnnexB2AVC(payload),
			}

		case StreamTypePCMATapo:
			p.Sequence++
			p.Timestamp += uint32(len(payload))

			pkt = &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    p.StreamType,
					SequenceNumber: p.Sequence,
					Timestamp:      p.Timestamp,
				},
				Payload: payload,
			}
		}

		p.Payload = nil

	case ModeStream:
		payload, i := p.decodeStream(p.Payload)
		if payload == nil {
			return
		}

		//log.Printf("[AVC] %v, len: %d", h264.Types(payload), len(payload))

		p.Payload = p.Payload[i:]

		pkt = &rtp.Packet{
			Header: rtp.Header{
				PayloadType: p.StreamType,
				Timestamp:   core.Now90000(),
			},
			Payload: payload,
		}

	default:
		p.Payload = nil
	}

	return
}

func ParseTime(b []byte) uint32 {
	return (uint32(b[0]&0x0E) << 29) | (uint32(b[1]) << 22) | (uint32(b[2]&0xFE) << 14) | (uint32(b[3]) << 7) | (uint32(b[4]) >> 1)
}

func GetMedia(pkt *rtp.Packet) *core.Media {
	var codec *core.Codec
	var kind string

	switch pkt.PayloadType {
	case StreamTypeH264:
		codec = &core.Codec{
			Name:        core.CodecH264,
			ClockRate:   90000,
			PayloadType: core.PayloadTypeRAW,
			FmtpLine:    h264.GetFmtpLine(pkt.Payload),
		}
		kind = core.KindVideo

	case StreamTypePCMATapo:
		codec = &core.Codec{
			Name:      core.CodecPCMA,
			ClockRate: 8000,
		}
		kind = core.KindAudio

	default:
		return nil
	}

	return &core.Media{
		Kind:      kind,
		Direction: core.DirectionRecvonly,
		Codecs:    []*core.Codec{codec},
	}
}
