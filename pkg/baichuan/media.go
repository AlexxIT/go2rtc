package baichuan

import (
	"encoding/binary"
	"fmt"
	"time"
)

const (
	mediaVideoHeaderSize = 24

	mediaInfoV1    = 0x31303031
	mediaInfoV2    = 0x32303031
	mediaIFrameMin = 0x63643030
	mediaIFrameMax = 0x63643039
	mediaPFrameMin = 0x63643130
	mediaPFrameMax = 0x63643139
	mediaAAC       = 0x62773530
	mediaAACV2     = 0x62773531
	mediaADPCM     = 0x62773130
	mediaAlignment = 8
	codecH264      = 0x34363248
	codecH265      = 0x35363248
)

type MediaKind uint8

const (
	MediaInfo MediaKind = iota + 1
	MediaVideoI
	MediaVideoP
	MediaAAC
	MediaADPCM
)

type MediaPacket struct {
	Kind      MediaKind
	Codec     string
	Data      []byte
	Timestamp uint32
	WallTime  time.Time
	Width     uint32
	Height    uint32
	FPS       uint8
	InfoV2    bool
}

type MediaParser struct {
	limits Limits
	buf    []byte
	scan   int
}

func NewMediaParser(limits Limits) (*MediaParser, error) {
	limits, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	return &MediaParser{limits: limits}, nil
}

func (p *MediaParser) Append(data []byte) ([]MediaPacket, error) {
	if uint64(len(p.buf))+uint64(len(data)) > uint64(p.limits.MaxMediaBuffer) {
		return nil, fmt.Errorf("baichuan: media buffer exceeds limit %d", p.limits.MaxMediaBuffer)
	}
	return p.appendOwned(append([]byte(nil), data...))
}

// appendOwned may retain data and is only for payloads exclusively owned by the caller.
func (p *MediaParser) appendOwned(data []byte) ([]MediaPacket, error) {
	return p.appendOwnedTo(data, nil)
}

func (p *MediaParser) appendOwnedTo(data []byte, packets []MediaPacket) ([]MediaPacket, error) {
	if uint64(len(p.buf))+uint64(len(data)) > uint64(p.limits.MaxMediaBuffer) {
		return nil, fmt.Errorf("baichuan: media buffer exceeds limit %d", p.limits.MaxMediaBuffer)
	}
	if len(p.buf) == 0 {
		p.buf = data
	} else {
		p.buf = append(p.buf, data...)
	}

	packets = packets[:0]
	for len(p.buf) >= 4 {
		packet, size, complete, err := p.parsePacket(p.buf)
		if err != nil {
			return packets, err
		}
		if !complete {
			break
		}
		p.scan = 0
		p.buf = p.buf[size:]
		packets = append(packets, packet)
	}
	if len(p.buf) == 0 {
		p.buf = nil
	}
	return packets, nil
}

func knownMediaMagic(magic uint32) bool {
	return magic == mediaInfoV1 || magic == mediaInfoV2 ||
		magic >= mediaIFrameMin && magic <= mediaIFrameMax ||
		magic >= mediaPFrameMin && magic <= mediaPFrameMax ||
		magic == mediaAAC || magic == mediaAACV2 || magic == mediaADPCM
}

func (p *MediaParser) parsePacket(b []byte) (MediaPacket, int, bool, error) {
	magic := binary.LittleEndian.Uint32(b)
	switch {
	case magic == mediaInfoV1 || magic == mediaInfoV2:
		if len(b) < 32 {
			return MediaPacket{}, 0, false, nil
		}
		if size := binary.LittleEndian.Uint32(b[4:8]); size != 32 {
			return MediaPacket{}, 0, false, fmt.Errorf("baichuan: invalid media info size %d", size)
		}
		return MediaPacket{
			Kind: MediaInfo, Width: binary.LittleEndian.Uint32(b[8:12]),
			Height: binary.LittleEndian.Uint32(b[12:16]), FPS: b[17], InfoV2: magic == mediaInfoV2,
		}, 32, true, nil
	case magic >= mediaIFrameMin && magic <= mediaIFrameMax:
		return p.parseVideo(b, true)
	case magic >= mediaPFrameMin && magic <= mediaPFrameMax:
		return p.parseVideo(b, false)
	case magic == mediaAAC || magic == mediaAACV2:
		return p.parseAudio(b, MediaAAC)
	case magic == mediaADPCM:
		return p.parseAudio(b, MediaADPCM)
	default:
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: unknown media magic %#x", magic)
	}
}

func (p *MediaParser) parseVideo(b []byte, keyframe bool) (MediaPacket, int, bool, error) {
	const headerSize = mediaVideoHeaderSize
	if len(b) < headerSize {
		return MediaPacket{}, 0, false, nil
	}
	var codec string
	switch binary.LittleEndian.Uint32(b[4:8]) {
	case codecH264:
		codec = "H264"
	case codecH265:
		codec = "H265"
	default:
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: unsupported video codec %q", b[4:8])
	}
	payload := binary.LittleEndian.Uint32(b[8:12])
	extra := binary.LittleEndian.Uint32(b[12:16])
	if keyframe {
		return p.parseIFrame(b, codec, payload, extra)
	}
	// Observed P-frames honor their declared size; only I-frames require bounded boundary recovery.
	dataStart := uint64(headerSize) + uint64(extra)
	plain := dataStart + uint64(payload)
	total := plain + uint64(padding(payload))
	if total > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: video frame %d exceeds limit %d", total, p.limits.MaxMediaFrame)
	}
	size, complete, err := mediaBoundary(b, plain, total, p.limits.MaxResync)
	if err != nil {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: video payload %d extra header %d: %w", payload, extra, err)
	}
	if !complete {
		return MediaPacket{}, 0, false, nil
	}
	packet := MediaPacket{
		Kind: MediaVideoP, Codec: codec, Data: b[int(dataStart):int(plain)],
		Timestamp: binary.LittleEndian.Uint32(b[16:20]),
	}
	return packet, size, true, nil
}

func (p *MediaParser) parseIFrame(b []byte, codec string, payload, extra uint32) (MediaPacket, int, bool, error) {
	const headerSize = mediaVideoHeaderSize
	dataStart := uint64(headerSize) + uint64(extra)
	plain := dataStart + uint64(payload)
	declared := plain + uint64(padding(payload))
	if declared > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: video frame %d exceeds limit %d", declared, p.limits.MaxMediaFrame)
	}
	size, complete := p.findVideoBoundary(b, int(dataStart), declared)
	if !complete {
		if uint64(len(b)) >= declared+uint64(p.limits.MaxResync)+mediaVideoHeaderSize {
			return MediaPacket{}, 0, false, fmt.Errorf("baichuan: no keyframe boundary near declared size %d extra header %d", declared, extra)
		}
		return MediaPacket{}, 0, false, nil
	}
	if uint64(size) > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: video frame %d exceeds limit %d", size, p.limits.MaxMediaFrame)
	}
	payloadEnd := size
	if uint64(size) == declared {
		payloadEnd = int(plain)
	}
	var wallTime time.Time
	if extra >= 4 {
		wallTime = time.Unix(int64(binary.LittleEndian.Uint32(b[24:28])), 0).UTC()
	}
	return MediaPacket{
		Kind: MediaVideoI, Codec: codec, Data: b[int(dataStart):payloadEnd],
		Timestamp: binary.LittleEndian.Uint32(b[16:20]),
		WallTime:  wallTime,
	}, size, true, nil
}

func (p *MediaParser) findVideoBoundary(b []byte, headerSize int, declared uint64) (int, bool) {
	if offset := int(declared); offset <= len(b)-8 && plausibleMediaHeader(b[offset:]) {
		return offset, true
	}
	start := int(declared) - int(p.limits.MaxResync)
	if start < headerSize {
		start = headerSize
	}
	if p.scan < start {
		p.scan = start
	}
	end := int(declared) + int(p.limits.MaxResync)
	if end > len(b)-8 {
		end = len(b) - 8
	}
	best, pending := -1, -1
	for i := p.scan; i <= end; i++ {
		if plausibleMediaHeader(b[i:]) && (best < 0 || abs(i-int(declared)) < abs(best-int(declared))) {
			best = i
		} else if pending < 0 && partialVideoHeader(b[i:]) {
			pending = i
		}
	}
	if pending >= 0 {
		p.scan = pending
	} else if end >= p.scan {
		p.scan = end + 1
	}
	return best, best >= 0
}

func partialVideoHeader(b []byte) bool {
	if len(b) >= mediaVideoHeaderSize {
		return false
	}
	magic := binary.LittleEndian.Uint32(b)
	return magic >= mediaIFrameMin && magic <= mediaIFrameMax || magic >= mediaPFrameMin && magic <= mediaPFrameMax
}

func plausibleMediaHeader(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	magic := binary.LittleEndian.Uint32(b)
	switch {
	case magic == mediaInfoV1 || magic == mediaInfoV2:
		return binary.LittleEndian.Uint32(b[4:8]) == 32
	case magic >= mediaIFrameMin && magic <= mediaIFrameMax || magic >= mediaPFrameMin && magic <= mediaPFrameMax:
		if len(b) < mediaVideoHeaderSize {
			return false
		}
		codec := binary.LittleEndian.Uint32(b[4:8])
		return codec == codecH264 || codec == codecH265
	case magic == mediaAAC || magic == mediaAACV2 || magic == mediaADPCM:
		return binary.LittleEndian.Uint16(b[4:6]) == binary.LittleEndian.Uint16(b[6:8])
	default:
		return false
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (p *MediaParser) parseAudio(b []byte, kind MediaKind) (MediaPacket, int, bool, error) {
	if len(b) < 8 {
		return MediaPacket{}, 0, false, nil
	}
	payload16 := binary.LittleEndian.Uint16(b[4:6])
	if payload16 != binary.LittleEndian.Uint16(b[6:8]) {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: inconsistent audio payload sizes")
	}
	payload := uint32(payload16)
	plain := uint64(8) + uint64(payload)
	total := plain + uint64(padding(payload))
	if total > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: audio frame %d exceeds limit %d", total, p.limits.MaxMediaFrame)
	}
	size, complete, err := mediaBoundary(b, plain, total, p.limits.MaxResync)
	if err != nil {
		return MediaPacket{}, 0, false, err
	}
	if !complete {
		return MediaPacket{}, 0, false, nil
	}
	start := 8
	if kind == MediaADPCM {
		if payload < 4 {
			return MediaPacket{}, 0, false, fmt.Errorf("baichuan: invalid ADPCM payload size %d", payload)
		}
		if binary.LittleEndian.Uint16(b[8:10]) != 0x0100 {
			return MediaPacket{}, 0, false, fmt.Errorf("baichuan: invalid ADPCM marker")
		}
		start += 4
		payload -= 4
	}
	return MediaPacket{Kind: kind, Data: b[start : start+int(payload)]}, size, true, nil
}

func mediaBoundary(b []byte, plain, padded uint64, resync uint32) (int, bool, error) {
	if uint64(len(b)) < plain {
		return 0, false, nil
	}
	if plain == padded {
		return int(plain), true, nil
	}
	if uint64(len(b)) >= plain+4 && hasMediaMagic(b[plain:]) {
		return int(plain), true, nil
	}
	if uint64(len(b)) < padded {
		return 0, false, nil
	}
	if uint64(len(b)) == padded {
		for _, value := range b[plain:padded] {
			if value != 0 {
				return 0, false, nil
			}
		}
		return int(padded), true, nil
	}
	if uint64(len(b)) >= padded+4 && hasMediaMagic(b[padded:]) {
		return int(padded), true, nil
	}
	if uint64(len(b)) < padded+4 {
		return 0, false, nil
	}

	start := int(plain) - int(resync)
	if start < 4 {
		start = 4
	}
	end := int(padded) + int(resync)
	if end > len(b)-3 {
		end = len(b) - 3
	}
	for i := start; i < end; i++ {
		if hasMediaMagic(b[i:]) {
			return 0, false, fmt.Errorf("baichuan: media boundary differs from declared size by %d bytes (next magic %#x)",
				i-int(padded), binary.LittleEndian.Uint32(b[i:]))
		}
	}
	return 0, false, fmt.Errorf("baichuan: no media boundary after declared size %d", padded)
}

func padding(size uint32) uint32 {
	if rem := size % mediaAlignment; rem != 0 {
		return mediaAlignment - rem
	}
	return 0
}
