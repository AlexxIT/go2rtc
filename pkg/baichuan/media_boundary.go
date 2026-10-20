package baichuan

import "encoding/binary"

// Recovery lookahead covers bounded successor headers and codec prefixes.
const mediaRecoveryLookahead = 64

// confirmRecoveryBoundary returns -1 for invalid, 0 for valid, or the required
// candidate-relative byte count when more input is needed.
func confirmRecoveryBoundary(b []byte, maxFrame uint32) int {
	if len(b) < 4 {
		if wordPrefix(b, mediaInfoV1) || wordPrefix(b, mediaInfoV2) ||
			mediaMagicRangePrefix(b, mediaIFrameMin, mediaIFrameMax) ||
			mediaMagicRangePrefix(b, mediaPFrameMin, mediaPFrameMax) ||
			wordPrefix(b, mediaAAC) || wordPrefix(b, mediaAACV2) || wordPrefix(b, mediaADPCM) {
			return 4
		}
		return -1
	}
	// Compressed bytes can resemble media headers, so adjusted boundaries need valid payload prefixes.
	magic := binary.LittleEndian.Uint32(b)
	switch {
	case magic == mediaInfoV1 || magic == mediaInfoV2:
		end := len(b)
		if end > 8 {
			end = 8
		}
		if !wordPrefix(b[4:end], 32) {
			return -1
		}
		if len(b) < 32 {
			return 32
		}
		return 0
	case magic >= mediaIFrameMin && magic <= mediaIFrameMax || magic >= mediaPFrameMin && magic <= mediaPFrameMax:
		end := len(b)
		if end > 8 {
			end = 8
		}
		if !wordPrefix(b[4:end], codecH264) && !wordPrefix(b[4:end], codecH265) {
			return -1
		}
		if len(b) < mediaVideoHeaderSize {
			return mediaVideoHeaderSize
		}
		codec := binary.LittleEndian.Uint32(b[4:8])
		payload := binary.LittleEndian.Uint32(b[8:12])
		extra := binary.LittleEndian.Uint32(b[12:16])
		if codec != codecH264 && codec != codecH265 {
			return -1
		}
		total := uint64(mediaVideoHeaderSize) + uint64(extra) + uint64(payload) + uint64(padding(payload))
		if total > uint64(maxFrame) {
			return -1
		}
		return confirmAnnexB(b, int(uint64(mediaVideoHeaderSize)+uint64(extra)), int(payload), codec == codecH265)
	case magic == mediaAAC || magic == mediaAACV2 || magic == mediaADPCM:
		return confirmRecoveryAudio(b, magic, maxFrame)
	default:
		return -1
	}
}

func confirmRecoveryAudio(b []byte, magic, maxFrame uint32) int {
	if len(b) >= 7 && b[4] != b[6] {
		return -1
	}
	if len(b) < 8 {
		return 8
	}
	payload := binary.LittleEndian.Uint16(b[4:6])
	if payload != binary.LittleEndian.Uint16(b[6:8]) {
		return -1
	}
	total := uint64(8) + uint64(payload) + uint64(padding(uint32(payload)))
	if total > uint64(maxFrame) {
		return -1
	}
	if magic == mediaADPCM {
		if payload < 4 {
			return -1
		}
		if len(b) < 10 {
			return 10
		}
		if binary.LittleEndian.Uint16(b[8:10]) != 0x0100 {
			return -1
		}
		return 0
	}
	if payload < 7 {
		return -1
	}
	if len(b) < 15 {
		if !partialADTS(b[8:], payload) {
			return -1
		}
		return 15
	}
	if adtsLength(b[8:15]) != uint32(payload) {
		return -1
	}
	return 0
}

func confirmAnnexB(b []byte, start, payload int, h265 bool) int {
	nalSize := 1
	if h265 {
		nalSize = 2
	}
	if payload < 3+nalSize {
		return -1
	}
	if len(b) < start+3 {
		return start + 3
	}
	startCode := 3
	if b[start] != 0 || b[start+1] != 0 {
		return -1
	}
	if b[start+2] == 0 {
		if payload < 4+nalSize {
			return -1
		}
		if len(b) < start+4 {
			return start + 4
		}
		if b[start+3] != 1 {
			return -1
		}
		startCode = 4
	} else if b[start+2] != 1 {
		return -1
	}
	nal := start + startCode
	if len(b) < nal+nalSize {
		return nal + nalSize
	}
	if h265 {
		if b[nal]&0x80 != 0 || (b[nal]>>1)&0x3f >= 48 || b[nal+1]&7 == 0 {
			return -1
		}
	} else if b[nal]&0x80 != 0 || b[nal]&0x1f == 0 || b[nal]&0x1f > 23 {
		return -1
	}
	return 0
}
