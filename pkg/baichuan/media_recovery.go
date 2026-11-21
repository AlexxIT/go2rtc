package baichuan

import "encoding/binary"

// Failed frames get a bounded search, independent of the larger allocation cap.
const mediaRecoveryWindow = 1 << 20

type mediaRecoveryRange struct{ start, end int }

func (p *MediaParser) recoveryWindows(start int, declared uint64, keyframe bool) ([2]mediaRecoveryRange, int) {
	end := start + mediaRecoveryWindow
	var ranges [2]mediaRecoveryRange
	if p.limits.MaxResync != defaultMaxResync {
		end = start + int(p.limits.MaxResync)
		if keyframe {
			end = int(declared) + int(p.limits.MaxResync)
			if lower := int(declared) - int(p.limits.MaxResync); lower > start {
				start = lower
			}
		}
	}
	ranges[0] = mediaRecoveryRange{start, end}
	if keyframe && p.limits.MaxResync == defaultMaxResync {
		near := mediaRecoveryRange{int(declared) - int(defaultMaxResync), int(declared) + 2*int(defaultMaxResync)}
		if near.start < start {
			near.start = start
		}
		if near.start <= end+1 {
			if near.end > end {
				ranges[0].end = near.end
			}
		} else {
			ranges[1] = near
		}
	}
	end = ranges[0].end
	if ranges[1].end > end {
		end = ranges[1].end
	}
	for i := range ranges {
		if ranges[i].end > int(p.limits.MaxMediaFrame) {
			ranges[i].end = int(p.limits.MaxMediaFrame)
		}
		if ranges[i].end > int(p.limits.MaxMediaBuffer) {
			ranges[i].end = int(p.limits.MaxMediaBuffer)
		}
	}
	deadline := end + mediaRecoveryLookahead
	if deadline > int(p.limits.MaxMediaBuffer) {
		deadline = int(p.limits.MaxMediaBuffer)
	}
	return ranges, deadline
}

func (p *MediaParser) recoverMediaBoundary(b []byte, rejectedVideo bool) (int, bool) {
	start := 1
	magic := binary.LittleEndian.Uint32(b)
	if rejectedVideo {
		// A rejected header cannot supply a trusted payload offset.
		start = mediaVideoHeaderSize
	} else if magic >= mediaPFrameMin && magic <= mediaPFrameMax && len(b) >= mediaVideoHeaderSize {
		if p.limits.MaxResync != defaultMaxResync && binary.LittleEndian.Uint32(b[8:12]) > p.limits.MaxResync {
			return 0, false
		}
		start = mediaVideoHeaderSize + int(binary.LittleEndian.Uint32(b[12:16]))
	} else if magic == mediaAAC || magic == mediaAACV2 || magic == mediaADPCM {
		start = 8
	}
	ranges, deadline := p.recoveryWindows(start, 0, false)
	return p.scanRecoveryBoundary(b, ranges, deadline)
}

func (p *MediaParser) scanRecoveryBoundary(b []byte, ranges [2]mediaRecoveryRange, deadline int) (int, bool) {
	available := len(b)
	if available > deadline {
		available = deadline
	}
	for _, window := range ranges {
		if window.end == 0 {
			continue
		}
		if p.scan < window.start {
			p.scan = window.start
		}
		for p.scan <= window.end && p.scan < available {
			i := p.scan
			need := p.confirmRecoveryAnchor(b[i:available])
			if need == 0 {
				return i, false
			}
			if need > 0 && i+need <= deadline {
				return 0, available < deadline
			}
			p.scan++
		}
		if p.scan <= window.end {
			return 0, available < deadline
		}
	}
	return 0, false
}

func (p *MediaParser) confirmRecoveryAnchor(b []byte) int {
	need := confirmRecoveryBoundary(b, p.limits.MaxMediaFrame)
	if need == 0 {
		magic := binary.LittleEndian.Uint32(b)
		if magic == mediaAAC || magic == mediaAACV2 || magic == mediaADPCM {
			return p.confirmRecoveryAudioEnd(b)
		}
	}
	return need
}

// Audio needs its complete body, zero padding, and a corroborating successor.
// Only fixed headers and endpoints are inspected again when more bytes arrive.
func (p *MediaParser) confirmRecoveryAudioEnd(b []byte) int {
	plain := 8 + int(binary.LittleEndian.Uint16(b[4:6]))
	if len(b) < plain {
		return plain
	}
	padded := plain + int(padding(uint32(plain-8)))
	for _, next := range [2]int{plain, padded} {
		if next > plain {
			limit := next
			if limit > len(b) {
				limit = len(b)
			}
			for _, value := range b[plain:limit] {
				if value != 0 {
					return -1
				}
			}
			if next > len(b) {
				return next
			}
		}
		need := confirmRecoveryBoundary(b[next:], p.limits.MaxMediaFrame)
		if need >= 0 {
			if need > 0 {
				return next + need
			}
			return 0
		}
	}
	return -1
}

func mediaMagicRangePrefix(b []byte, min, max uint32) bool {
	for i, value := range b {
		if value < byte(min>>(8*i)) || value > byte(max>>(8*i)) {
			return false
		}
	}
	return true
}

func wordPrefix(b []byte, word uint32) bool {
	for i, value := range b {
		if i == 4 || value != byte(word>>(8*i)) {
			return false
		}
	}
	return true
}

func partialADTS(b []byte, payload uint16) bool {
	if len(b) > 0 && b[0] != 0xff || len(b) > 1 && b[1]&0xf6 != 0xf0 || len(b) > 2 && b[2]&0x3c == 0x3c {
		return false
	}
	if len(b) > 1 && (b[1]&1 == 0 && payload < 9 || b[1]&1 != 0 && payload < 7) {
		return false
	}
	if len(b) > 3 && uint32(payload)>>11 != uint32(b[3]&3) {
		return false
	}
	if len(b) > 4 && uint32(payload)&0x1ff8 != (uint32(b[3]&3)<<11|uint32(b[4])<<3) {
		return false
	}
	if len(b) > 5 && uint32(payload) != (uint32(b[3]&3)<<11|uint32(b[4])<<3|uint32(b[5])>>5) {
		return false
	}
	return len(b) < 7
}
