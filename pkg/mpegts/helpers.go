package mpegts

import (
	"bytes"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
	"time"
)

const (
	PacketSize = 188
	SyncByte   = 0x47
)

const (
	StreamTypeAAC      = 0x0F
	StreamTypeH264     = 0x1B
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

		if p.StreamType == StreamTypeH264 {
			if bytes.HasPrefix(b, []byte{0, 0, 0, 1, h264.NALUTypeAUD}) {
				p.Mode = ModeStream
				b = b[5:]
			}
		}

		if p.Mode == ModeUnknown {
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

		p.Payload = p.Payload[minHeaderSize+optSize:]

		switch p.StreamType {
		case StreamTypeH264:
			var ts uint32

			const hasPTS = 0b1000_0000
			if flags&hasPTS != 0 {
				ts = uint32(ParseTime(p.Payload[minHeaderSize:]))
			}

			pkt = &rtp.Packet{
				Header: rtp.Header{
					PayloadType: p.StreamType,
					Timestamp:   ts,
				},
				Payload: h264.AnnexB2AVC(p.Payload),
			}

		case StreamTypePCMATapo:
			p.Sequence++
			p.Timestamp += uint32(len(p.Payload))

			pkt = &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    p.StreamType,
					SequenceNumber: p.Sequence,
					Timestamp:      p.Timestamp,
				},
				Payload: p.Payload,
			}
		}

		p.Payload = nil

	case ModeStream:
		i := bytes.Index(p.Payload, []byte{0, 0, 0, 1, h264.NALUTypeAUD})
		if i < 0 {
			return
		}
		if i2 := IndexFrom(p.Payload, []byte{0, 0, 1}, i); i2 < 0 && i2 > 9 {
			return
		}

		pkt = &rtp.Packet{
			Header: rtp.Header{
				PayloadType: p.StreamType,
				Timestamp:   uint32(time.Duration(time.Now().UnixNano()) * 90000 / time.Second),
			},
			Payload: DecodeAnnex3B(p.Payload[:i]),
		}

		p.Payload = p.Payload[i+5:]

	default:
		p.Payload = nil
	}

	return
}

func ParseTime(b []byte) time.Duration {
	ts := (uint64(b[0]) >> 1 & 0x7 << 30) | (uint64(b[1]) << 22) | (uint64(b[2]) >> 1 & 0x7F << 15) | (uint64(b[3]) << 7) | (uint64(b[4]) >> 1 & 0x7F)
	return time.Duration(ts)
}

func GetMedia(pkt *rtp.Packet) *streamer.Media {
	var codec *streamer.Codec
	var kind string

	switch pkt.PayloadType {
	case StreamTypeH264:
		codec = &streamer.Codec{
			Name:        streamer.CodecH264,
			ClockRate:   90000,
			PayloadType: streamer.PayloadTypeRAW,
			FmtpLine:    h264.GetFmtpLine(pkt.Payload),
		}
		kind = streamer.KindVideo

	case StreamTypePCMATapo:
		codec = &streamer.Codec{
			Name:      streamer.CodecPCMA,
			ClockRate: 8000,
		}
		kind = streamer.KindAudio

	default:
		return nil
	}

	return &streamer.Media{
		Kind:      kind,
		Direction: streamer.DirectionSendonly,
		Codecs:    []*streamer.Codec{codec},
	}
}

func DecodeAnnex3B(annexb []byte) (avc []byte) {
	// depends on AU delimeter size
	i0 := bytes.Index(annexb, []byte{0, 0, 1})
	if i0 < 0 || i0 > 9 {
		return nil
	}

	annexb = annexb[i0+3:] // skip first separator
	i0 = 0

	for {
		// search next separato
		iN := IndexFrom(annexb, []byte{0, 0, 1}, i0)
		if iN < 0 {
			break
		}

		// move i0 to next AU
		if i0 = iN + 3; i0 >= len(annexb) {
			break
		}

		// check if AU type valid
		octet := annexb[i0]
		const forbiddenZeroBit = 0x80
		if octet&forbiddenZeroBit == 0 {
			const nalUnitType = 0x1F
			switch octet & nalUnitType {
			case h264.NALUTypePFrame, h264.NALUTypeIFrame, h264.NALUTypeSPS, h264.NALUTypePPS:
				// add AU in AVC format
				avc = append(avc, byte(iN>>24), byte(iN>>16), byte(iN>>8), byte(iN))
				avc = append(avc, annexb[:iN]...)

				// cut search to next AU start
				annexb = annexb[i0:]
				i0 = 0
			}
		}
	}

	size := len(annexb)
	avc = append(avc, byte(size>>24), byte(size>>16), byte(size>>8), byte(size))
	return append(avc, annexb...)
}

func IndexFrom(b []byte, sep []byte, from int) int {
	if from > 0 {
		if from < len(b) {
			if i := bytes.Index(b[from:], sep); i >= 0 {
				return from + i
			}
		}
		return -1
	}

	return bytes.Index(b, sep)
}
