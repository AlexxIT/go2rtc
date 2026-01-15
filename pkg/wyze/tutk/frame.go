package tutk

import (
	"encoding/binary"
	"encoding/hex"
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

const (
	ResTierLow  uint8 = 1 // 360P/SD
	ResTierHigh uint8 = 4 // HD/2K
)

const (
	Bitrate360P uint8 = 30
	BitrateHD   uint8 = 100
	Bitrate2K   uint8 = 200
)

const FrameInfoSize = 40

// FrameInfo - Wyze extended FRAMEINFO (40 bytes at end of packet)
// Video: 40 bytes, Audio: 16 bytes (uses same struct, fields 16+ are zero)
//
// Offset  Size  Field
// 0-1     2     CodecID     - 0x4E=H264, 0x7B=H265, 0x90=AAC_WYZE
// 2       1     Flags       - Video: 1=Keyframe, 0=P-frame | Audio: sample rate/bits/channels
// 3       1     CamIndex    - Camera index
// 4       1     OnlineNum   - Online number
// 5       1     FPS         - Framerate (e.g. 20)
// 6       1     ResTier     - Video: 1=Low(360P), 4=High(HD/2K) | Audio: 0
// 7       1     Bitrate     - Video: 30=360P, 100=HD, 200=2K | Audio: 1
// 8-11    4     Timestamp   - Timestamp (increases ~50000/frame for 20fps video)
// 12-15   4     SessionID   - Session marker (constant per stream)
// 16-19   4     PayloadSize - Frame payload size in bytes
// 20-23   4     FrameNo     - Global frame number
// 24-35   12    DeviceID    - MAC address (ASCII) - video only
// 36-39   4     Padding     - Always 0 - video only
type FrameInfo struct {
	CodecID     uint16 // 0-1
	Flags       uint8  // 2
	CamIndex    uint8  // 3
	OnlineNum   uint8  // 4
	FPS         uint8  // 5: Framerate
	ResTier     uint8  // 6: Resolution tier (1=Low, 4=High)
	Bitrate     uint8  // 7: Bitrate index (30=360P, 100=HD, 200=2K)
	Timestamp   uint32 // 8-11: Timestamp
	SessionID   uint32 // 12-15: Session marker (constant)
	PayloadSize uint32 // 16-19: Payload size
	FrameNo     uint32 // 20-23: Frame number
}

func (fi *FrameInfo) IsKeyframe() bool {
	return fi.Flags == 0x01
}

func (fi *FrameInfo) Resolution() string {
	switch fi.Bitrate {
	case Bitrate360P:
		return "360P"
	case BitrateHD:
		return "HD"
	case Bitrate2K:
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
		FPS:         fi[5],
		ResTier:     fi[6],
		Bitrate:     fi[7],
		Timestamp:   binary.LittleEndian.Uint32(fi[8:]),
		SessionID:   binary.LittleEndian.Uint32(fi[12:]),
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

		if pktIdxOrMarker == 0x0028 && (IsEndFrame(frameType) || hdr.PktTotal == 1) {
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

		if pktIdxOrMarker == 0x0028 && (IsEndFrame(frameType) || hdr.PktTotal == 1) {
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

type channelState struct {
	frameNo    uint32     // current frame being assembled
	pktTotal   uint16     // expected total packets
	waitSeq    uint16     // next expected packet index (0, 1, 2, ...)
	waitData   []byte     // accumulated payload data
	frameInfo  *FrameInfo // frame info (from end packet)
	hasStarted bool       // received first packet of frame
	lastPktIdx uint16     // last received packet index (for OOO detection)
}

func (cs *channelState) reset() {
	cs.frameNo = 0
	cs.pktTotal = 0
	cs.waitSeq = 0
	cs.waitData = cs.waitData[:0]
	cs.frameInfo = nil
	cs.hasStarted = false
	cs.lastPktIdx = 0
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

const tsWrapPeriod uint32 = 1000000

type FrameHandler struct {
	channels  map[byte]*channelState
	lastRawTS uint32
	accumUS   uint64
	firstTS   bool
	output    chan *Packet
	verbose   bool
}

func NewFrameHandler(verbose bool) *FrameHandler {
	return &FrameHandler{
		channels: make(map[byte]*channelState),
		output:   make(chan *Packet, 128),
		verbose:  verbose,
	}
}

func (h *FrameHandler) Recv() <-chan *Packet {
	return h.output
}

func (h *FrameHandler) Close() {
	close(h.output)
}

func (h *FrameHandler) updateTimestamp(rawTS uint32) uint64 {
	if !h.firstTS {
		h.firstTS = true
		h.lastRawTS = rawTS
		return 0
	}

	var delta uint32
	if rawTS >= h.lastRawTS {
		delta = rawTS - h.lastRawTS
	} else {
		// Wrapped: delta = (wrap - last) + new
		delta = (tsWrapPeriod - h.lastRawTS) + rawTS
	}

	h.accumUS += uint64(delta)
	h.lastRawTS = rawTS

	return h.accumUS
}

func (h *FrameHandler) Handle(data []byte) {
	hdr := ParsePacketHeader(data)
	if hdr == nil {
		return
	}

	payload, fi := h.extractPayload(data, hdr.Channel)
	if payload == nil {
		return
	}

	if h.verbose {
		fiStr := ""
		if hdr.HasFrameInfo {
			fiStr = " +FI"
		}
		fmt.Printf("[RX] ch=0x%02x type=0x%02x #%d pkt=%d/%d data=%dB%s\n",
			hdr.Channel, hdr.FrameType,
			hdr.FrameNo, hdr.PktIdx, hdr.PktTotal, len(payload), fiStr)
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
	cs := h.channels[channel]
	if cs == nil {
		cs = &channelState{}
		h.channels[channel] = cs
	}

	// New frame number - reset and start fresh
	if hdr.FrameNo != cs.frameNo {
		// Check if previous frame was incomplete
		if cs.hasStarted && cs.waitSeq < cs.pktTotal {
			fmt.Printf("[DROP] ch=0x%02x #%d INCOMPLETE: got %d/%d pkts\n",
				channel, cs.frameNo, cs.waitSeq, cs.pktTotal)
		}
		cs.reset()
		cs.frameNo = hdr.FrameNo
		cs.pktTotal = hdr.PktTotal
	}

	// Sequential check: if packet index doesn't match expected, reset (data loss)
	if hdr.PktIdx != cs.waitSeq {
		fmt.Printf("[OOO] ch=0x%02x #%d frameType=0x%02x pktTotal=%d expected pkt %d, got %d - reset\n",
			channel, hdr.FrameNo, hdr.FrameType, hdr.PktTotal, cs.waitSeq, hdr.PktIdx)
		cs.reset()
		return
	}

	// First packet - mark as started
	if cs.waitSeq == 0 {
		cs.hasStarted = true
	}

	// Append payload (simple sequential accumulation)
	cs.waitData = append(cs.waitData, payload...)
	cs.waitSeq++

	// Store frame info if present
	if fi != nil {
		cs.frameInfo = fi
	}

	// Check if frame is complete
	if cs.waitSeq == cs.pktTotal && cs.frameInfo != nil {
		h.emitVideo(channel, cs)
		cs.reset()
	}
}

func (h *FrameHandler) emitVideo(channel byte, cs *channelState) {
	fi := cs.frameInfo

	// Size validation
	if fi.PayloadSize > 0 && uint32(len(cs.waitData)) != fi.PayloadSize {
		fmt.Printf("[SIZE] ch=0x%02x #%d mismatch: expected %d, got %d\n",
			channel, cs.frameNo, fi.PayloadSize, len(cs.waitData))
		return
	}

	if len(cs.waitData) == 0 {
		return
	}

	accumUS := h.updateTimestamp(fi.Timestamp)
	rtpTS := uint32(accumUS * 90000 / 1000000)

	// Copy payload (buffer will be reused)
	payload := make([]byte, len(cs.waitData))
	copy(payload, cs.waitData)

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
			frameType = "KEY"
		}
		fmt.Printf("[OK] ch=0x%02x #%d %s %s size=%d\n",
			channel, fi.FrameNo, CodecName(fi.CodecID), frameType, len(payload))
		fmt.Printf("  [0-1]codec=0x%x(%s) [2]flags=0x%x [3]=%d [4]=%d\n",
			fi.CodecID, CodecName(fi.CodecID), fi.Flags, fi.CamIndex, fi.OnlineNum)
		fmt.Printf("  [5]=%d [6]=%d [7]=%d [8-11]ts=%d\n",
			fi.FPS, fi.ResTier, fi.Bitrate, fi.Timestamp)
		fmt.Printf("  [12-15]=0x%x [16-19]payload=%d [20-23]frameNo=%d\n",
			fi.SessionID, fi.PayloadSize, fi.FrameNo)
		fmt.Printf("  rtp_ts=%d accum_us=%d\n", rtpTS, accumUS)
		fmt.Printf("  hex: %s\n", dumpHex(fi))
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

	accumUS := h.updateTimestamp(fi.Timestamp)
	rtpTS := uint32(accumUS * uint64(sampleRate) / 1000000)

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
		bits := 8
		if fi.Flags&0x02 != 0 {
			bits = 16
		}
		fmt.Printf("[OK] Audio #%d %s size=%d\n",
			fi.FrameNo, AudioCodecName(fi.CodecID), len(payload))
		fmt.Printf("  [0-1]codec=0x%x(%s) [2]flags=0x%x(%dHz/%dbit/%dch)\n",
			fi.CodecID, AudioCodecName(fi.CodecID), fi.Flags, sampleRate, bits, channels)
		fmt.Printf("  [8-11]ts=%d [12-15]=0x%x rtp_ts=%d\n",
			fi.Timestamp, fi.SessionID, rtpTS)
		fmt.Printf("  hex: %s\n", dumpHex(fi))
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

func dumpHex(fi *FrameInfo) string {
	b := make([]byte, FrameInfoSize)
	binary.LittleEndian.PutUint16(b[0:], fi.CodecID)
	b[2] = fi.Flags
	b[3] = fi.CamIndex
	b[4] = fi.OnlineNum
	b[5] = fi.FPS
	b[6] = fi.ResTier
	b[7] = fi.Bitrate
	binary.LittleEndian.PutUint32(b[8:], fi.Timestamp)
	binary.LittleEndian.PutUint32(b[12:], fi.SessionID)
	binary.LittleEndian.PutUint32(b[16:], fi.PayloadSize)
	binary.LittleEndian.PutUint32(b[20:], fi.FrameNo)
	// Bytes 24-39 are DeviceID and Padding (not stored in struct)

	hexStr := hex.EncodeToString(b)
	formatted := ""
	for i := 0; i < len(hexStr); i += 2 {
		if i > 0 {
			formatted += " "
		}
		formatted += hexStr[i : i+2]
	}
	return formatted
}
