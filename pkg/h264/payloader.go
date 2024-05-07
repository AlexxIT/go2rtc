package h264

import "encoding/binary"

// Payloader payloads H264 packets
type Payloader struct {
	IsAVC     bool
	stapANalu []byte
}

const (
	stapaNALUType  = 24
	fuaNALUType    = 28
	fubNALUType    = 29
	spsNALUType    = 7
	ppsNALUType    = 8
	audNALUType    = 9
	fillerNALUType = 12

	fuaHeaderSize = 2
	//stapaHeaderSize     = 1
	//stapaNALULengthSize = 2

	naluTypeBitmask   = 0x1F
	naluRefIdcBitmask = 0x60
	//fuStartBitmask    = 0x80
	//fuEndBitmask      = 0x40

	outputStapAHeader = 0x78
)

//func annexbNALUStartCode() []byte { return []byte{0x00, 0x00, 0x00, 0x01} }

func EmitNalus(nals []byte, isAVC bool, emit func([]byte)) {
	if !isAVC {
		nextInd := func(nalu []byte, start int) (indStart int, indLen int) {
			zeroCount := 0

			for i, b := range nalu[start:] {
				if b == 0 {
					zeroCount++
					continue
				} else if b == 1 {
					if zeroCount >= 2 {
						return start + i - zeroCount, zeroCount + 1
					}
				}
				zeroCount = 0
			}
			return -1, -1
		}

		nextIndStart, nextIndLen := nextInd(nals, 0)
		if nextIndStart == -1 {
			emit(nals)
		} else {
			for nextIndStart != -1 {
				prevStart := nextIndStart + nextIndLen
				nextIndStart, nextIndLen = nextInd(nals, prevStart)
				if nextIndStart != -1 {
					emit(nals[prevStart:nextIndStart])
				} else {
					// Emit until end of stream, no end indicator found
					emit(nals[prevStart:])
				}
			}
		}
	} else {
		for {
			n := uint32(len(nals))
			if n < 4 {
				break
			}
			end := 4 + binary.BigEndian.Uint32(nals)
			if n < end {
				break
			}
			emit(nals[4:end])
			nals = nals[end:]
		}
	}
}

// Payload fragments a H264 packet across one or more byte arrays
func (p *Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	var payloads [][]byte
	if len(payload) == 0 {
		return payloads
	}

	EmitNalus(payload, p.IsAVC, func(nalu []byte) {
		if len(nalu) == 0 {
			return
		}

		naluType := nalu[0] & naluTypeBitmask
		naluRefIdc := nalu[0] & naluRefIdcBitmask

		switch naluType {
		case audNALUType, fillerNALUType:
			return
		case spsNALUType, ppsNALUType:
			if p.stapANalu == nil {
				p.stapANalu = []byte{outputStapAHeader}
			}
			p.stapANalu = append(p.stapANalu, byte(len(nalu)>>8), byte(len(nalu)))
			p.stapANalu = append(p.stapANalu, nalu...)
			return
		}

		if p.stapANalu != nil {
			// Pack current NALU with SPS and PPS as STAP-A
			// Supports multiple PPS in a row
			if len(p.stapANalu) <= int(mtu) {
				payloads = append(payloads, p.stapANalu)
			}
			p.stapANalu = nil
		}

		// Single NALU
		if len(nalu) <= int(mtu) {
			out := make([]byte, len(nalu))
			copy(out, nalu)
			payloads = append(payloads, out)
			return
		}

		// FU-A
		maxFragmentSize := int(mtu) - fuaHeaderSize

		// The FU payload consists of fragments of the payload of the fragmented
		// NAL unit so that if the fragmentation unit payloads of consecutive
		// FUs are sequentially concatenated, the payload of the fragmented NAL
		// unit can be reconstructed.  The NAL unit type octet of the fragmented
		// NAL unit is not included as such in the fragmentation unit payload,
		// 	but rather the information of the NAL unit type octet of the
		// fragmented NAL unit is conveyed in the F and NRI fields of the FU
		// indicator octet of the fragmentation unit and in the type field of
		// the FU header.  An FU payload MAY have any number of octets and MAY
		// be empty.

		naluData := nalu
		// According to the RFC, the first octet is skipped due to redundant information
		naluDataIndex := 1
		naluDataLength := len(nalu) - naluDataIndex
		naluDataRemaining := naluDataLength

		if min(maxFragmentSize, naluDataRemaining) <= 0 {
			return
		}

		for naluDataRemaining > 0 {
			currentFragmentSize := min(maxFragmentSize, naluDataRemaining)
			out := make([]byte, fuaHeaderSize+currentFragmentSize)

			// +---------------+
			// |0|1|2|3|4|5|6|7|
			// +-+-+-+-+-+-+-+-+
			// |F|NRI|  Type   |
			// +---------------+
			out[0] = fuaNALUType
			out[0] |= naluRefIdc

			// +---------------+
			// |0|1|2|3|4|5|6|7|
			// +-+-+-+-+-+-+-+-+
			// |S|E|R|  Type   |
			// +---------------+

			out[1] = naluType
			if naluDataRemaining == naluDataLength {
				// Set start bit
				out[1] |= 1 << 7
			} else if naluDataRemaining-currentFragmentSize == 0 {
				// Set end bit
				out[1] |= 1 << 6
			}

			copy(out[fuaHeaderSize:], naluData[naluDataIndex:naluDataIndex+currentFragmentSize])
			payloads = append(payloads, out)

			naluDataRemaining -= currentFragmentSize
			naluDataIndex += currentFragmentSize
		}
	})

	return payloads
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
