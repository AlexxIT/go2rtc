package tutk

import (
	"encoding/binary"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/aac"
)

const (
	FrameTypeStart     uint8 = 0x08 // Extended start (36-byte header)
	FrameTypeStartAlt  uint8 = 0x09 // StartAlt (36-byte header)
	FrameTypeCont      uint8 = 0x00 // Continuation (28-byte header)
	FrameTypeContAlt   uint8 = 0x04 // Continuation alt
	FrameTypeEndSingle uint8 = 0x01 // Single-packet frame (28-byte)
	FrameTypeEndMulti  uint8 = 0x05 // Multi-packet end (28-byte)
	FrameTypeEndExt    uint8 = 0x0d // Extended end (36-byte)
)

const (
	ChannelIVideo uint8 = 0x05
	ChannelAudio  uint8 = 0x03
	ChannelPVideo uint8 = 0x07
)

// Resolution constants
const (
	ResolutionUnknown = 0
	ResolutionSD      = 1
	Resolution360P    = 2
	Resolution2K      = 4
)

const FrameInfoSize = 40

// FrameInfo - Wyze extended FRAMEINFO (40 bytes at end of packet)
type FrameInfo struct {
	CodecID     uint16
	Flags       uint8
	CamIndex    uint8
	OnlineNum   uint8
	Framerate   uint8
	FrameSize   uint8
	Bitrate     uint8
	TimestampUS uint32
	Timestamp   uint32
	PayloadSize uint32
	FrameNo     uint32
}

func (fi *FrameInfo) IsKeyframe() bool {
	return fi.Flags == 0x01
}

func (fi *FrameInfo) Resolution() string {
	switch fi.FrameSize {
	case ResolutionSD:
		return "SD"
	case Resolution360P:
		return "360P"
	case Resolution2K:
		return "2K"
	default:
		return "unknown"
	}
}

func (fi *FrameInfo) SampleRate() uint32 {
	idx := (fi.Flags >> 2) & 0x0F
	return uint32(SampleRateValue(idx))
}

func (fi *FrameInfo) Channels() uint8 {
	if fi.Flags&0x01 == 1 {
		return 2
	}
	return 1
}

func (fi *FrameInfo) IsVideo() bool {
	return IsVideoCodec(fi.CodecID)
}

func (fi *FrameInfo) IsAudio() bool {
	return IsAudioCodec(fi.CodecID)
}

func ParseFrameInfo(data []byte) *FrameInfo {
	if len(data) < FrameInfoSize {
		return nil
	}

	offset := len(data) - FrameInfoSize
	fi := data[offset:]

	return &FrameInfo{
		CodecID:     binary.LittleEndian.Uint16(fi),
		Flags:       fi[2],
		CamIndex:    fi[3],
		OnlineNum:   fi[4],
		Framerate:   fi[5],
		FrameSize:   fi[6],
		Bitrate:     fi[7],
		TimestampUS: binary.LittleEndian.Uint32(fi[8:]),
		Timestamp:   binary.LittleEndian.Uint32(fi[12:]),
		PayloadSize: binary.LittleEndian.Uint32(fi[16:]),
		FrameNo:     binary.LittleEndian.Uint32(fi[20:]),
	}
}

type Packet struct {
	Channel    uint8
	Codec      uint16
	Timestamp  uint32
	Payload    []byte
	IsKeyframe bool
	FrameNo    uint32
	SampleRate uint32
	Channels   uint8
}

func (p *Packet) IsVideo() bool {
	return p.Channel == ChannelIVideo || p.Channel == ChannelPVideo
}

func (p *Packet) IsAudio() bool {
	return p.Channel == ChannelAudio
}

type PacketHeader struct {
	Channel      byte
	FrameType    byte
	HeaderSize   int
	FrameNo      uint32
	PktIdx       uint16
	PktTotal     uint16
	PayloadSize  uint16
	HasFrameInfo bool
}

func ParsePacketHeader(data []byte) *PacketHeader {
	if len(data) < 28 {
		return nil
	}

	frameType := data[1]
	hdr := &PacketHeader{
		Channel:   data[0],
		FrameType: frameType,
	}

	switch frameType {
	case FrameTypeStart, FrameTypeStartAlt, FrameTypeEndExt:
		hdr.HeaderSize = 36
	default:
		hdr.HeaderSize = 28
	}

	if len(data) < hdr.HeaderSize {
		return nil
	}

	if hdr.HeaderSize == 28 {
		hdr.PktTotal = binary.LittleEndian.Uint16(data[12:])
		pktIdxOrMarker := binary.LittleEndian.Uint16(data[14:])
		hdr.PayloadSize = binary.LittleEndian.Uint16(data[16:])
		hdr.FrameNo = binary.LittleEndian.Uint32(data[24:])

		if IsEndFrame(frameType) && pktIdxOrMarker == 0x0028 {
			hdr.HasFrameInfo = true
			if hdr.PktTotal > 0 {
				hdr.PktIdx = hdr.PktTotal - 1
			}
		} else {
			hdr.PktIdx = pktIdxOrMarker
		}
	} else {
		hdr.PktTotal = binary.LittleEndian.Uint16(data[20:])
		pktIdxOrMarker := binary.LittleEndian.Uint16(data[22:])
		hdr.PayloadSize = binary.LittleEndian.Uint16(data[24:])
		hdr.FrameNo = binary.LittleEndian.Uint32(data[32:])

		if IsEndFrame(frameType) && pktIdxOrMarker == 0x0028 {
			hdr.HasFrameInfo = true
			if hdr.PktTotal > 0 {
				hdr.PktIdx = hdr.PktTotal - 1
			}
		} else {
			hdr.PktIdx = pktIdxOrMarker
		}
	}

	return hdr
}

func IsStartFrame(frameType uint8) bool {
	return frameType == FrameTypeStart || frameType == FrameTypeStartAlt
}

func IsEndFrame(frameType uint8) bool {
	return frameType == FrameTypeEndSingle ||
		frameType == FrameTypeEndMulti ||
		frameType == FrameTypeEndExt
}

func IsContinuationFrame(frameType uint8) bool {
	return frameType == FrameTypeCont || frameType == FrameTypeContAlt
}

type FrameAssembler struct {
	FrameNo   uint32
	PktTotal  uint16
	Packets   map[uint16][]byte
	FrameInfo *FrameInfo
}

func ParseAudioParams(payload []byte, fi *FrameInfo) (sampleRate uint32, channels uint8) {
	if aac.IsADTS(payload) {
		codec := aac.ADTSToCodec(payload)
		if codec != nil {
			return codec.ClockRate, codec.Channels
		}
	}

	if fi != nil {
		return fi.SampleRate(), fi.Channels()
	}

	return 16000, 1
}

type FrameHandler struct {
	assemblers map[byte]*FrameAssembler
	baseTS     uint64
	output     chan *Packet
	verbose    bool
}

func NewFrameHandler(verbose bool) *FrameHandler {
	return &FrameHandler{
		assemblers: make(map[byte]*FrameAssembler),
		output:     make(chan *Packet, 128),
		verbose:    verbose,
	}
}

func (h *FrameHandler) Recv() <-chan *Packet {
	return h.output
}

func (h *FrameHandler) Close() {
	close(h.output)
}

func (h *FrameHandler) Handle(data []byte) {
	hdr := ParsePacketHeader(data)
	if hdr == nil {
		return
	}

	if h.verbose {
		h.logWireHeader(data, hdr)
	}

	payload, fi := h.extractPayload(data, hdr.Channel)
	if payload == nil {
		return
	}

	if h.verbose {
		h.logAVPacket(hdr.Channel, hdr.FrameType, payload, fi)
	}

	switch hdr.Channel {
	case ChannelAudio:
		h.handleAudio(payload, fi)
	case ChannelIVideo, ChannelPVideo:
		h.handleVideo(hdr.Channel, hdr, payload, fi)
	}
}

func (h *FrameHandler) extractPayload(data []byte, channel byte) ([]byte, *FrameInfo) {
	if len(data) < 2 {
		return nil, nil
	}

	frameType := data[1]

	headerSize := 28
	frameInfoSize := 0

	switch frameType {
	case FrameTypeStart:
		headerSize = 36
	case FrameTypeStartAlt:
		headerSize = 36
		if len(data) >= 22 {
			pktTotal := binary.LittleEndian.Uint16(data[20:])
			if pktTotal == 1 {
				frameInfoSize = FrameInfoSize
			}
		}
	case FrameTypeCont, FrameTypeContAlt:
		headerSize = 28
	case FrameTypeEndSingle, FrameTypeEndMulti:
		headerSize = 28
		frameInfoSize = FrameInfoSize
	case FrameTypeEndExt:
		headerSize = 36
		frameInfoSize = FrameInfoSize
	default:
		headerSize = 28
	}

	if len(data) < headerSize {
		return nil, nil
	}

	if frameInfoSize == 0 {
		return data[headerSize:], nil
	}

	if len(data) < headerSize+frameInfoSize {
		return data[headerSize:], nil
	}

	fi := ParseFrameInfo(data)

	validCodec := false
	switch channel {
	case ChannelIVideo, ChannelPVideo:
		validCodec = IsVideoCodec(fi.CodecID)
	case ChannelAudio:
		validCodec = IsAudioCodec(fi.CodecID)
	}

	if validCodec {
		payload := data[headerSize : len(data)-frameInfoSize]
		return payload, fi
	}

	return data[headerSize:], nil
}

func (h *FrameHandler) handleVideo(channel byte, hdr *PacketHeader, payload []byte, fi *FrameInfo) {
	asm := h.assemblers[channel]

	// Frame transition: new frame number = previous frame complete
	if asm != nil && hdr.FrameNo != asm.FrameNo {
		gotAll := uint16(len(asm.Packets)) == asm.PktTotal
		if gotAll && asm.FrameInfo != nil {
			h.assembleAndQueue(channel, asm)
		}
		asm = nil
	}

	// Create new assembler if needed
	if asm == nil {
		asm = &FrameAssembler{
			FrameNo:  hdr.FrameNo,
			PktTotal: hdr.PktTotal,
			Packets:  make(map[uint16][]byte, hdr.PktTotal),
		}
		h.assemblers[channel] = asm
	}

	// Store packet (copy payload - buffer is reused by worker)
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)
	asm.Packets[hdr.PktIdx] = payloadCopy

	if fi != nil {
		asm.FrameInfo = fi
	}

	// Check if frame is complete
	if uint16(len(asm.Packets)) == asm.PktTotal && asm.FrameInfo != nil {
		h.assembleAndQueue(channel, asm)
		delete(h.assemblers, channel)
	}
}

func (h *FrameHandler) assembleAndQueue(channel byte, asm *FrameAssembler) {
	fi := asm.FrameInfo

	// Assemble packets in correct order
	var payload []byte
	for i := uint16(0); i < asm.PktTotal; i++ {
		if pkt, ok := asm.Packets[i]; ok {
			payload = append(payload, pkt...)
		}
	}

	// Size validation
	if fi.PayloadSize > 0 && len(payload) != int(fi.PayloadSize) {
		return
	}

	if len(payload) == 0 {
		return
	}

	// Calculate RTP timestamp (90kHz for video) using relative timestamps
	absoluteTS := uint64(fi.Timestamp)*1000000 + uint64(fi.TimestampUS)
	if h.baseTS == 0 {
		h.baseTS = absoluteTS
	}
	relativeUS := absoluteTS - h.baseTS
	const clockRate uint64 = 90000
	rtpTS := uint32(relativeUS * clockRate / 1000000)

	pkt := &Packet{
		Channel:    channel,
		Payload:    payload,
		Codec:      fi.CodecID,
		Timestamp:  rtpTS,
		IsKeyframe: fi.IsKeyframe(),
		FrameNo:    fi.FrameNo,
	}

	if h.verbose {
		frameType := "P"
		if fi.IsKeyframe() {
			frameType = "I"
		}
		fmt.Printf("[VIDEO] #%d %s %s size=%d rtp=%d\n",
			fi.FrameNo, CodecName(fi.CodecID), frameType, len(payload), rtpTS)
	}

	h.queue(pkt)
}

func (h *FrameHandler) handleAudio(payload []byte, fi *FrameInfo) {
	if len(payload) == 0 || fi == nil {
		return
	}

	var sampleRate uint32
	var channels uint8

	switch fi.CodecID {
	case AudioCodecAACRaw, AudioCodecAACADTS, AudioCodecAACLATM, AudioCodecAACWyze:
		sampleRate, channels = ParseAudioParams(payload, fi)
	default:
		sampleRate = fi.SampleRate()
		channels = fi.Channels()
	}

	// Calculate RTP timestamp using relative timestamps (shared baseTS for A/V sync)
	absoluteTS := uint64(fi.Timestamp)*1000000 + uint64(fi.TimestampUS)
	if h.baseTS == 0 {
		h.baseTS = absoluteTS
	}
	relativeUS := absoluteTS - h.baseTS
	clockRate := uint64(sampleRate)
	rtpTS := uint32(relativeUS * clockRate / 1000000)

	pkt := &Packet{
		Channel:    ChannelAudio,
		Payload:    payload,
		Codec:      fi.CodecID,
		Timestamp:  rtpTS,
		SampleRate: sampleRate,
		Channels:   channels,
		FrameNo:    fi.FrameNo,
	}

	if h.verbose {
		fmt.Printf("[AUDIO] #%d %s size=%d rate=%d ch=%d rtp=%d\n",
			fi.FrameNo, AudioCodecName(fi.CodecID), len(payload), sampleRate, channels, rtpTS)
	}

	h.queue(pkt)
}

func (h *FrameHandler) queue(pkt *Packet) {
	select {
	case h.output <- pkt:
	default:
		// Queue full - drop oldest
		select {
		case <-h.output:
		default:
		}
		h.output <- pkt
	}
}

func (h *FrameHandler) logWireHeader(data []byte, hdr *PacketHeader) {
	fmt.Printf("[WIRE] ch=0x%02x type=0x%02x len=%d pkt=%d/%d frame=%d\n",
		hdr.Channel, hdr.FrameType, len(data), hdr.PktIdx, hdr.PktTotal, hdr.FrameNo)
	fmt.Printf("       RAW[0..35]: ")
	for i := 0; i < 36 && i < len(data); i++ {
		fmt.Printf("%02x ", data[i])
	}
	fmt.Printf("\n")
}

func (h *FrameHandler) logAVPacket(channel, frameType byte, payload []byte, fi *FrameInfo) {
	fmt.Printf("[AV] ch=0x%02x type=0x%02x len=%d", channel, frameType, len(payload))
	if fi != nil {
		fmt.Printf(" fi={codec=0x%04x flags=0x%02x ts=%d}", fi.CodecID, fi.Flags, fi.Timestamp)
	}
	fmt.Printf("\n")
}
