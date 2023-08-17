package mpegts

import (
	"github.com/AlexxIT/go2rtc/pkg/aac"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

const (
	PacketSize = 188
	SyncByte   = 0x47 // Uppercase G
)

// https://en.wikipedia.org/wiki/Program-specific_information#Elementary_stream_types
const (
	metadataType       = 0
	StreamTypePrivate  = 0x06 // PCMU or PCMA or FLAC from FFmpeg
	StreamTypeAAC      = 0x0F
	StreamTypeH264     = 0x1B
	StreamTypeH265     = 0x24
	StreamTypePCMATapo = 0x90
)

// PES - Packetized Elementary Stream
type PES struct {
	StreamType byte
	StreamID   byte
	Payload    []byte
	Size       int
	PTS        uint32 // PTS always 90000Hz

	Sequence uint16

	decodeStream func([]byte) ([]byte, int)
}

func (p *PES) SetBuffer(size uint16, b []byte) {
	p.Payload = make([]byte, 0, size)
	p.Payload = append(p.Payload, b...)
	p.Size = int(size)
}

func (p *PES) AppendBuffer(b []byte) {
	p.Payload = append(p.Payload, b...)
}

func (p *PES) GetPacket() (pkt *rtp.Packet) {
	switch p.StreamType {
	case StreamTypeH264, StreamTypeH265:
		pkt = &rtp.Packet{
			Header: rtp.Header{
				PayloadType: p.StreamType,
				Timestamp:   p.PTS,
			},
			Payload: annexb.EncodeToAVCC(p.Payload, false),
		}

	case StreamTypeAAC:
		p.Sequence++

		pkt = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    p.StreamType,
				SequenceNumber: p.Sequence,
				Timestamp:      p.PTS,
			},
			Payload: aac.ADTStoRTP(p.Payload),
		}

	case StreamTypePCMATapo:
		p.Sequence++
		p.PTS += uint32(len(p.Payload))

		pkt = &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    p.StreamType,
				SequenceNumber: p.Sequence,
				Timestamp:      p.PTS,
			},
			Payload: p.Payload,
		}
	}

	p.Payload = nil

	return
}

func StreamType(codec *core.Codec) uint8 {
	switch codec.Name {
	case core.CodecH264:
		return StreamTypeH264
	case core.CodecH265:
		return StreamTypeH265
	case core.CodecAAC:
		return StreamTypeAAC
	case core.CodecPCMA:
		return StreamTypePCMATapo
	}
	return 0
}

// PTSToTimestamp - convert PTS from 90000 to custom clock rate
func PTSToTimestamp(pts, clockRate uint32) uint32 {
	if clockRate == 90000 {
		return pts
	}
	return uint32(uint64(pts) * uint64(clockRate) / 90000)
}
