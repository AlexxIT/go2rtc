package tutk

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

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

const frameInfoSize = 40

// FrameInfo - Wyze extended FRAMEINFO (40 bytes at end of packet),
// matching wyzecam's FrameInfoStruct. Some devices (e.g. the WYZEDB3
// doorbell) send the 48-byte FrameInfo3Struct variant instead (marker
// 0x0030), which appends a face-detection box; bytes 0-23 are identical,
// so both are parsed front-aligned here and the extension is ignored.
// Video: 40/48 bytes, Audio: 16 bytes (same struct, fields 16+ are zero)
//
// Offset  Size  Field
// 0-1     2     CodecID       - 0x4E=H264, 0x7B=H265, 0x90=AAC_WYZE
// 2       1     Flags         - Video: 1=Keyframe, 0=P-frame | Audio: sample rate/bits/channels
// 3       1     CamIndex      - Camera index
// 4       1     OnlineNum     - Online number
// 5       1     FPS           - Framerate (e.g. 20)
// 6       1     ResTier       - Video: 1=Low(360P), 4=High(HD/2K) | Audio: 0
// 7       1     Bitrate       - Video: 30=360P, 100=HD, 200=2K | Audio: 1
// 8-11    4     TimestampFrac - Sub-second time component in device units (~us; increases ~50000/frame at 20fps). Some firmware (e.g. WYZEDB3 4.25.1.333) leaves it 0 - see tsTracker's wall-clock fallback.
// 12-15   4     TimestampSec  - Unix epoch seconds (ticks 1/s; previously mislabeled SessionID)
// 16-19   4     PayloadSize   - Frame payload size in bytes
// 20-23   4     FrameNo       - Global frame number
// 24-35   12    DeviceID      - MAC address (ASCII) - video only
// 36-39   4     PlayToken     - n_play_token - video only
// 40-47   8     FaceBox       - FrameInfo3Struct only: face_pos_x/y, face_width/height (4x u16)
type FrameInfo struct {
	CodecID       byte   // 0 (only low byte used)
	Flags         uint8  // 2
	CamIndex      uint8  // 3
	OnlineNum     uint8  // 4
	FPS           uint8  // 5: Framerate
	ResTier       uint8  // 6: Resolution tier (1=Low, 4=High)
	Bitrate       uint8  // 7: Bitrate index (30=360P, 100=HD, 200=2K)
	TimestampFrac uint32 // 8-11: sub-second time component (0 on some firmware)
	TimestampSec  uint32 // 12-15: Unix epoch seconds
	PayloadSize   uint32 // 16-19: Payload size
	FrameNo       uint32 // 20-23: Frame number
}

func (fi *FrameInfo) IsKeyframe() bool {
	return fi.Flags == 0x01
}

func (fi *FrameInfo) SampleRate() uint32 {
	idx := (fi.Flags >> 2) & 0x0F
	if idx < uint8(len(sampleRates)) {
		return sampleRates[idx]
	}
	return 16000
}

func (fi *FrameInfo) Channels() uint8 {
	if fi.Flags&0x01 == 1 {
		return 2
	}
	return 1
}

func ParseFrameInfo(data []byte) *FrameInfo {
	return parseFrameInfo(data, frameInfoSize)
}

func parseFrameInfo(data []byte, fiSize int) *FrameInfo {
	if fiSize < frameInfoSize {
		fiSize = frameInfoSize
	}
	if len(data) < fiSize {
		return nil
	}

	offset := len(data) - fiSize
	fi := data[offset:]

	return &FrameInfo{
		CodecID:       fi[0],
		Flags:         fi[2],
		CamIndex:      fi[3],
		OnlineNum:     fi[4],
		FPS:           fi[5],
		ResTier:       fi[6],
		Bitrate:       fi[7],
		TimestampFrac: binary.LittleEndian.Uint32(fi[8:]),
		TimestampSec:  binary.LittleEndian.Uint32(fi[12:]),
		PayloadSize:   binary.LittleEndian.Uint32(fi[16:]),
		FrameNo:       binary.LittleEndian.Uint32(fi[20:]),
	}
}

type Packet struct {
	Channel    uint8
	Codec      byte
	Timestamp  uint32
	Payload    []byte
	IsKeyframe bool
	FrameNo    uint32
	SampleRate uint32
	Channels   uint8
}

type PacketHeader struct {
	Channel       byte
	FrameType     byte
	HeaderSize    int
	FrameNo       uint32
	PktIdx        uint16
	PktTotal      uint16
	PayloadSize   uint16
	HasFrameInfo  bool
	FrameInfoSize int // actual FrameInfo size in bytes (may differ from standard 40)
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

		if isFrameInfoMarker(pktIdxOrMarker, hdr.PktTotal, frameType) {
			hdr.HasFrameInfo = true
			hdr.FrameInfoSize = int(pktIdxOrMarker)
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

		if isFrameInfoMarker(pktIdxOrMarker, hdr.PktTotal, frameType) {
			hdr.HasFrameInfo = true
			hdr.FrameInfoSize = int(pktIdxOrMarker)
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

// isFrameInfoMarker returns true when the field at [14:16] (or [22:24] for 36-byte
// headers) is a FrameInfo size marker rather than a packet index.
// Known marker values are matched explicitly: 0x0028 (40 bytes, standard) and
// 0x0030 (48 bytes, e.g. doorbells). As a backstop, any value >= PktTotal is
// impossible as a packet index and is also treated as a size marker (this
// heuristic alone is insufficient for frames larger than the marker value).
func isFrameInfoMarker(val, pktTotal uint16, frameType uint8) bool {
	if !IsEndFrame(frameType) && pktTotal != 1 {
		return false
	}
	return val == 0x0028 || val == 0x0030 || (pktTotal > 0 && val >= pktTotal)
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

const tsWrapPeriod uint32 = 1000000

// tsDeadThreshold - consecutive zero deltas of the device timestamp before
// concluding the firmware never populates it (e.g. WYZEDB3 4.25.1.333 sends
// TimestampFrac as 0 on every frame) and pacing from the wall clock instead.
const tsDeadThreshold = 3

type tsTracker struct {
	lastRawTS uint32
	accumUS   uint64
	firstTS   bool
	lastWall  time.Time
	zeroRun   uint8
	synth     bool
}

func (t *tsTracker) update(rawTS uint32) uint64 {
	now := time.Now()

	if !t.firstTS {
		t.firstTS = true
		t.lastRawTS = rawTS
		t.lastWall = now
		return 0
	}

	var delta uint32
	if rawTS >= t.lastRawTS {
		delta = rawTS - t.lastRawTS
	} else {
		// Wrapped: delta = (wrap - last) + new
		delta = (tsWrapPeriod - t.lastRawTS) + rawTS
	}

	// Dead-source detection: a camera that never advances its timestamp
	// would freeze the whole timeline at 0, stalling MSE/WebRTC/HLS
	// consumers that pace by RTP timestamps (video still decodes, so
	// RTSP/ffmpeg users never notice). Latch to wall-clock pacing.
	if !t.synth {
		if delta == 0 {
			if t.zeroRun++; t.zeroRun >= tsDeadThreshold {
				t.synth = true
				fmt.Printf("[TS] device timestamps dead (raw=%d), pacing from wall clock\n", rawTS)
			}
		} else {
			t.zeroRun = 0
		}
	}

	if t.synth {
		// time.Now() carries a monotonic reading, so Sub is immune to
		// NTP steps; the guard keeps the timeline monotonic regardless.
		if us := now.Sub(t.lastWall).Microseconds(); us > 0 {
			delta = uint32(us)
		} else {
			delta = 0
		}
	}

	t.accumUS += uint64(delta)
	t.lastRawTS = rawTS
	t.lastWall = now

	return t.accumUS
}

type FrameHandler struct {
	channels map[byte]*channelState
	videoTS  tsTracker
	audioTS  tsTracker
	output   chan *Packet
	verbose  bool
	closed   bool
	closeMu  sync.Mutex
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
	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return
	}
	h.closed = true
	close(h.output)
}

func (h *FrameHandler) Handle(data []byte) {
	hdr := ParsePacketHeader(data)
	if hdr == nil {
		return
	}

	payload, fi := h.extractPayload(data, hdr.Channel, hdr.FrameInfoSize)
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

func (h *FrameHandler) extractPayload(data []byte, channel byte, fiSizeHint int) ([]byte, *FrameInfo) {
	if len(data) < 2 {
		return nil, nil
	}

	frameType := data[1]

	headerSize := 28
	fiSize := 0

	switch frameType {
	case FrameTypeStart:
		headerSize = 36
	case FrameTypeStartAlt:
		headerSize = 36
		if len(data) >= 22 {
			pktTotal := binary.LittleEndian.Uint16(data[20:])
			if pktTotal == 1 {
				fiSize = frameInfoSize
			}
		}
	case FrameTypeCont, FrameTypeContAlt:
		headerSize = 28
	case FrameTypeEndSingle, FrameTypeEndMulti:
		headerSize = 28
		fiSize = frameInfoSize
		if fiSizeHint > fiSize {
			fiSize = fiSizeHint
		}
	case FrameTypeEndExt:
		headerSize = 36
		fiSize = frameInfoSize
		if fiSizeHint > fiSize {
			fiSize = fiSizeHint
		}
	default:
		headerSize = 28
	}

	if len(data) < headerSize {
		return nil, nil
	}

	if fiSize == 0 {
		return data[headerSize:], nil
	}

	if len(data) < headerSize+fiSize {
		return data[headerSize:], nil
	}

	fi := parseFrameInfo(data, fiSize)

	validCodec := false
	switch channel {
	case ChannelIVideo, ChannelPVideo:
		validCodec = IsVideoCodec(fi.CodecID)
	case ChannelAudio:
		validCodec = IsAudioCodec(fi.CodecID)
	}

	if validCodec {
		payload := data[headerSize : len(data)-fiSize]
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

	// If packet index doesn't match expected, reset (data loss)
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

	cs.waitData = append(cs.waitData, payload...)
	cs.waitSeq++

	// Store frame info if present
	if fi != nil {
		cs.frameInfo = fi
	}

	// Check if frame is complete
	if cs.waitSeq != cs.pktTotal || cs.frameInfo == nil {
		return
	}

	fi = cs.frameInfo
	defer cs.reset()

	if fi.PayloadSize > 0 && uint32(len(cs.waitData)) != fi.PayloadSize {
		fmt.Printf("[SIZE] ch=0x%02x #%d mismatch: expected %d, got %d\n",
			channel, cs.frameNo, fi.PayloadSize, len(cs.waitData))
		return
	}

	if len(cs.waitData) == 0 {
		return
	}

	accumUS := h.videoTS.update(fi.TimestampFrac)
	rtpTS := uint32(accumUS * 90000 / 1000000)

	pkt := &Packet{
		Channel:    channel,
		Payload:    append([]byte{}, cs.waitData...),
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
		fmt.Printf("[OK] ch=0x%02x #%d codec=0x%02x %s size=%d\n",
			channel, fi.FrameNo, fi.CodecID, frameType, len(pkt.Payload))
		fmt.Printf("  [0-1]codec=0x%02x [2]flags=0x%x [3]=%d [4]=%d\n",
			fi.CodecID, fi.Flags, fi.CamIndex, fi.OnlineNum)
		fmt.Printf("  [5]=%d [6]=%d [7]=%d [8-11]ts=%d\n",
			fi.FPS, fi.ResTier, fi.Bitrate, fi.TimestampFrac)
		fmt.Printf("  [12-15]sec=%d [16-19]payload=%d [20-23]frameNo=%d\n",
			fi.TimestampSec, fi.PayloadSize, fi.FrameNo)
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
	case CodecAACRaw, CodecAACADTS, CodecAACLATM, CodecAACAlt:
		sampleRate, channels = parseAudioParams(payload, fi)
	default:
		sampleRate = fi.SampleRate()
		channels = fi.Channels()
	}

	accumUS := h.audioTS.update(fi.TimestampFrac)
	rtpTS := uint32(accumUS * uint64(sampleRate) / 1000000)

	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)

	pkt := &Packet{
		Channel:    ChannelAudio,
		Payload:    payloadCopy,
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
		fmt.Printf("[OK] Audio #%d codec=0x%02x size=%d\n",
			fi.FrameNo, fi.CodecID, len(payload))
		fmt.Printf("  [0-1]codec=0x%02x [2]flags=0x%x(%dHz/%dbit/%dch)\n",
			fi.CodecID, fi.Flags, sampleRate, bits, channels)
		fmt.Printf("  [8-11]ts=%d [12-15]=0x%x rtp_ts=%d\n",
			fi.TimestampFrac, fi.TimestampSec, rtpTS)
		fmt.Printf("  hex: %s\n", dumpHex(fi))
	}

	h.queue(pkt)
}

func (h *FrameHandler) queue(pkt *Packet) {
	h.closeMu.Lock()
	defer h.closeMu.Unlock()

	if h.closed {
		return
	}

	select {
	case h.output <- pkt:
	default:
		// Queue full - drop oldest
		select {
		case <-h.output:
		default:
		}
		select {
		case h.output <- pkt:
		default:
			// Queue still full, drop this packet
		}
	}
}

func parseAudioParams(payload []byte, fi *FrameInfo) (sampleRate uint32, channels uint8) {
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

func dumpHex(fi *FrameInfo) string {
	b := make([]byte, frameInfoSize)
	b[0] = fi.CodecID
	b[1] = 0 // High byte (unused)
	b[2] = fi.Flags
	b[3] = fi.CamIndex
	b[4] = fi.OnlineNum
	b[5] = fi.FPS
	b[6] = fi.ResTier
	b[7] = fi.Bitrate
	binary.LittleEndian.PutUint32(b[8:], fi.TimestampFrac)
	binary.LittleEndian.PutUint32(b[12:], fi.TimestampSec)
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
