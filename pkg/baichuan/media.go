package baichuan

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const (
	mediaVideoHeaderSize = 24
	maxMediaPackets      = 4096

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
	// MediaDiscontinuity requires video consumers to wait for a valid keyframe.
	MediaDiscontinuity
)

type MediaPacket struct {
	Kind       MediaKind
	Codec      string
	Data       []byte
	Timestamp  uint32
	WallTime   time.Time
	Width      uint32
	Height     uint32
	FPS        uint8
	InfoV2     bool
	ShortVideo bool // A recovered video boundary preceded the declared payload end.
}

type MediaParser struct {
	limits Limits
	buf    []byte
	scan   int
}

var errMediaBoundaryMismatch = errors.New("baichuan: media boundary differs from declared size")
var errNoKeyframeBoundary = errors.New("baichuan: no keyframe boundary")
var errMediaFraming = errors.New("baichuan: invalid media framing")
var errMediaVideoHeader = fmt.Errorf("%w: invalid video header", errMediaFraming)

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
			if errors.Is(err, errMediaBoundaryMismatch) || errors.Is(err, errMediaFraming) {
				if offset, pending := p.recoverMediaBoundary(p.buf, errors.Is(err, errMediaVideoHeader)); offset > 0 {
					packet, size, complete, err = MediaPacket{Kind: MediaDiscontinuity}, offset, true, nil
				} else if pending {
					break
				}
			}
			if err != nil {
				return packets, err
			}
		}
		if !complete {
			break
		}
		if len(packets) >= maxMediaPackets {
			return packets, fmt.Errorf("baichuan: media packet count exceeds limit %d", maxMediaPackets)
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
			return MediaPacket{}, 0, false, fmt.Errorf("%w: invalid media info size %d", errMediaFraming, size)
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
		return MediaPacket{}, 0, false, fmt.Errorf("%w: unknown media magic %#x", errMediaFraming, magic)
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
		return MediaPacket{}, 0, false, fmt.Errorf("%w: unsupported video codec %#x", errMediaVideoHeader, binary.LittleEndian.Uint32(b[4:8]))
	}
	payload := binary.LittleEndian.Uint32(b[8:12])
	extra := binary.LittleEndian.Uint32(b[12:16])
	if keyframe {
		return p.parseIFrame(b, codec, payload, extra)
	}
	// Healthy frames use their declared boundary without scanning payload bytes.
	dataStart := uint64(headerSize) + uint64(extra)
	plain := dataStart + uint64(payload)
	total := plain + uint64(padding(payload))
	if total > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("%w: video frame %d exceeds limit %d", errMediaVideoHeader, total, p.limits.MaxMediaFrame)
	}
	size, complete, err := mediaBoundary(b, plain, total)
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

func adtsLength(b []byte) uint32 {
	if len(b) < 7 || b[0] != 0xff || b[1]&0xf6 != 0xf0 || b[2]&0x3c == 0x3c {
		return 0
	}
	size := uint32(b[3]&3)<<11 | uint32(b[4])<<3 | uint32(b[5])>>5
	minimum := uint32(7)
	if b[1]&1 == 0 {
		minimum = 9
	}
	if size < minimum {
		return 0
	}
	return size
}

func (p *MediaParser) parseIFrame(b []byte, codec string, payload, extra uint32) (MediaPacket, int, bool, error) {
	const headerSize = mediaVideoHeaderSize
	dataStart := uint64(headerSize) + uint64(extra)
	plain := dataStart + uint64(payload)
	declared := plain + uint64(padding(payload))
	if declared > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("%w: video frame %d exceeds limit %d", errMediaVideoHeader, declared, p.limits.MaxMediaFrame)
	}
	size, pending := p.findVideoBoundary(b, int(dataStart), plain, declared)
	if size == 0 {
		if !pending {
			return MediaPacket{}, 0, false, fmt.Errorf("%w near declared size %d extra header %d", errNoKeyframeBoundary, declared, extra)
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
		Timestamp:  binary.LittleEndian.Uint32(b[16:20]),
		WallTime:   wallTime,
		ShortVideo: uint64(size) < plain,
	}, size, true, nil
}

func (p *MediaParser) findVideoBoundary(b []byte, dataStart int, plain, declared uint64) (int, bool) {
	if offset := int(declared); offset <= len(b)-8 && plausibleMediaHeader(b[offset:]) {
		return offset, false
	}
	if declared > uint64(len(b)) {
		return 0, true
	}
	ranges, deadline := p.recoveryWindows(dataStart, declared, true)
	available := len(b)
	if available > deadline {
		available = deadline
	}
	// Declared endpoints, including incomplete headers, precede payload recovery.
	if declared > uint64(available) {
		return 0, true
	}
	need := confirmRecoveryBoundary(b[declared:available], p.limits.MaxMediaFrame)
	if need == 0 {
		return int(declared), false
	}
	if need > 0 && declared+uint64(need) <= uint64(deadline) {
		return 0, true
	}
	if plain < declared {
		need = p.confirmRecoveryAnchor(b[plain:available])
		if need == 0 {
			return int(plain), false
		}
		if need > 0 && plain+uint64(need) <= uint64(deadline) {
			return 0, true
		}
	}
	return p.scanRecoveryBoundary(b, ranges, deadline)
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

func (p *MediaParser) parseAudio(b []byte, kind MediaKind) (MediaPacket, int, bool, error) {
	if len(b) < 8 {
		return MediaPacket{}, 0, false, nil
	}
	payload16 := binary.LittleEndian.Uint16(b[4:6])
	if payload16 != binary.LittleEndian.Uint16(b[6:8]) {
		return MediaPacket{}, 0, false, fmt.Errorf("%w: inconsistent audio payload sizes", errMediaFraming)
	}
	payload := uint32(payload16)
	plain := uint64(8) + uint64(payload)
	total := plain + uint64(padding(payload))
	if total > uint64(p.limits.MaxMediaFrame) {
		return MediaPacket{}, 0, false, fmt.Errorf("baichuan: audio frame %d exceeds limit %d", total, p.limits.MaxMediaFrame)
	}
	size, complete, err := mediaBoundary(b, plain, total)
	if err != nil {
		return MediaPacket{}, 0, false, err
	}
	if !complete {
		return MediaPacket{}, 0, false, nil
	}
	start := 8
	if kind == MediaADPCM {
		if payload < 4 {
			return MediaPacket{}, 0, false, fmt.Errorf("%w: invalid ADPCM payload size %d", errMediaFraming, payload)
		}
		if binary.LittleEndian.Uint16(b[8:10]) != 0x0100 {
			return MediaPacket{}, 0, false, fmt.Errorf("%w: invalid ADPCM marker", errMediaFraming)
		}
		start += 4
		payload -= 4
	}
	return MediaPacket{Kind: kind, Data: b[start : start+int(payload)]}, size, true, nil
}

func mediaBoundary(b []byte, plain, padded uint64) (int, bool, error) {
	if uint64(len(b)) < plain {
		return 0, false, nil
	}
	if plain == padded && (uint64(len(b)) < plain+4 || hasMediaMagic(b[plain:])) {
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

	return 0, false, fmt.Errorf("%w after declared size %d", errMediaBoundaryMismatch, padded)
}

func padding(size uint32) uint32 {
	if rem := size % mediaAlignment; rem != 0 {
		return mediaAlignment - rem
	}
	return 0
}
