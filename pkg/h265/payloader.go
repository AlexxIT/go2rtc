package h265

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"math"
)

//
// Network Abstraction Unit Header implementation
//

const (
	// sizeof(uint16)
	h265NaluHeaderSize = 2
	// https://datatracker.ietf.org/doc/html/rfc7798#section-4.4.2
	h265NaluAggregationPacketType = 48
	// https://datatracker.ietf.org/doc/html/rfc7798#section-4.4.3
	h265NaluFragmentationUnitType = 49
	// https://datatracker.ietf.org/doc/html/rfc7798#section-4.4.4
	h265NaluPACIPacketType = 50
)

// H265NALUHeader is a H265 NAL Unit Header
// https://datatracker.ietf.org/doc/html/rfc7798#section-1.1.4
// +---------------+---------------+
//
//	|0|1|2|3|4|5|6|7|0|1|2|3|4|5|6|7|
//	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//	|F|   Type    |  LayerID  | TID |
//	+-------------+-----------------+
type H265NALUHeader uint16

func newH265NALUHeader(highByte, lowByte uint8) H265NALUHeader {
	return H265NALUHeader((uint16(highByte) << 8) | uint16(lowByte))
}

// F is the forbidden bit, should always be 0.
func (h H265NALUHeader) F() bool {
	return (uint16(h) >> 15) != 0
}

// Type of NAL Unit.
func (h H265NALUHeader) Type() uint8 {
	// 01111110 00000000
	const mask = 0b01111110 << 8
	return uint8((uint16(h) & mask) >> (8 + 1))
}

// IsTypeVCLUnit returns whether or not the NAL Unit type is a VCL NAL unit.
func (h H265NALUHeader) IsTypeVCLUnit() bool {
	// Type is coded on 6 bits
	const msbMask = 0b00100000
	return (h.Type() & msbMask) == 0
}

// LayerID should always be 0 in non-3D HEVC context.
func (h H265NALUHeader) LayerID() uint8 {
	// 00000001 11111000
	const mask = (0b00000001 << 8) | 0b11111000
	return uint8((uint16(h) & mask) >> 3)
}

// TID is the temporal identifier of the NAL unit +1.
func (h H265NALUHeader) TID() uint8 {
	const mask = 0b00000111
	return uint8(uint16(h) & mask)
}

// IsAggregationPacket returns whether or not the packet is an Aggregation packet.
func (h H265NALUHeader) IsAggregationPacket() bool {
	return h.Type() == h265NaluAggregationPacketType
}

// IsFragmentationUnit returns whether or not the packet is a Fragmentation Unit packet.
func (h H265NALUHeader) IsFragmentationUnit() bool {
	return h.Type() == h265NaluFragmentationUnitType
}

// IsPACIPacket returns whether or not the packet is a PACI packet.
func (h H265NALUHeader) IsPACIPacket() bool {
	return h.Type() == h265NaluPACIPacketType
}

//
// Fragmentation Unit implementation
//

const (
	// sizeof(uint8)
	h265FragmentationUnitHeaderSize = 1
)

// H265FragmentationUnitHeader is a H265 FU Header
// +---------------+
// |0|1|2|3|4|5|6|7|
// +-+-+-+-+-+-+-+-+
// |S|E|  FuType   |
// +---------------+
type H265FragmentationUnitHeader uint8

// S represents the start of a fragmented NAL unit.
func (h H265FragmentationUnitHeader) S() bool {
	const mask = 0b10000000
	return ((h & mask) >> 7) != 0
}

// E represents the end of a fragmented NAL unit.
func (h H265FragmentationUnitHeader) E() bool {
	const mask = 0b01000000
	return ((h & mask) >> 6) != 0
}

// FuType MUST be equal to the field Type of the fragmented NAL unit.
func (h H265FragmentationUnitHeader) FuType() uint8 {
	const mask = 0b00111111
	return uint8(h) & mask
}

// Payloader payloads H265 packets
type Payloader struct {
	AddDONL         bool
	SkipAggregation bool
	donl            uint16
}

// Payload fragments a H265 packet across one or more byte arrays
func (p *Payloader) Payload(mtu uint16, payload []byte) [][]byte {
	var payloads [][]byte
	if len(payload) == 0 {
		return payloads
	}

	bufferedNALUs := make([][]byte, 0)
	aggregationBufferSize := 0

	flushBufferedNals := func() {
		if len(bufferedNALUs) == 0 {
			return
		}
		if len(bufferedNALUs) == 1 {
			// emit this as a single NALU packet
			nalu := bufferedNALUs[0]

			if p.AddDONL {
				buf := make([]byte, len(nalu)+2)

				// copy the NALU header to the payload header
				copy(buf[0:h265NaluHeaderSize], nalu[0:h265NaluHeaderSize])

				// copy the DONL into the header
				binary.BigEndian.PutUint16(buf[h265NaluHeaderSize:h265NaluHeaderSize+2], p.donl)

				// write the payload
				copy(buf[h265NaluHeaderSize+2:], nalu[h265NaluHeaderSize:])

				p.donl++

				payloads = append(payloads, buf)
			} else {
				// write the nalu directly to the payload
				payloads = append(payloads, nalu)
			}
		} else {
			// construct an aggregation packet
			aggregationPacketSize := aggregationBufferSize + 2
			buf := make([]byte, aggregationPacketSize)

			layerID := uint8(math.MaxUint8)
			tid := uint8(math.MaxUint8)
			for _, nalu := range bufferedNALUs {
				header := newH265NALUHeader(nalu[0], nalu[1])
				headerLayerID := header.LayerID()
				headerTID := header.TID()
				if headerLayerID < layerID {
					layerID = headerLayerID
				}
				if headerTID < tid {
					tid = headerTID
				}
			}

			binary.BigEndian.PutUint16(buf[0:2], (uint16(h265NaluAggregationPacketType)<<9)|(uint16(layerID)<<3)|uint16(tid))

			index := 2
			for i, nalu := range bufferedNALUs {
				if p.AddDONL {
					if i == 0 {
						binary.BigEndian.PutUint16(buf[index:index+2], p.donl)
						index += 2
					} else {
						buf[index] = byte(i - 1)
						index++
					}
				}
				binary.BigEndian.PutUint16(buf[index:index+2], uint16(len(nalu)))
				index += 2
				index += copy(buf[index:], nalu)
			}
			payloads = append(payloads, buf)
		}
		// clear the buffered NALUs
		bufferedNALUs = make([][]byte, 0)
		aggregationBufferSize = 0
	}

	h264.EmitNalus(payload, true, func(nalu []byte) {
		if len(nalu) == 0 {
			return
		}

		if len(nalu) <= int(mtu) {
			// this nalu fits into a single packet, either it can be emitted as
			// a single nalu or appended to the previous aggregation packet

			marginalAggregationSize := len(nalu) + 2
			if p.AddDONL {
				marginalAggregationSize += 1
			}

			if aggregationBufferSize+marginalAggregationSize > int(mtu) {
				flushBufferedNals()
			}
			bufferedNALUs = append(bufferedNALUs, nalu)
			aggregationBufferSize += marginalAggregationSize
			if p.SkipAggregation {
				// emit this immediately.
				flushBufferedNals()
			}
		} else {
			// if this nalu doesn't fit in the current mtu, it needs to be fragmented
			fuPacketHeaderSize := h265FragmentationUnitHeaderSize + 2 /* payload header size */
			if p.AddDONL {
				fuPacketHeaderSize += 2
			}

			// then, fragment the nalu
			maxFUPayloadSize := int(mtu) - fuPacketHeaderSize

			naluHeader := newH265NALUHeader(nalu[0], nalu[1])

			// the nalu header is omitted from the fragmentation packet payload
			nalu = nalu[h265NaluHeaderSize:]

			if maxFUPayloadSize == 0 || len(nalu) == 0 {
				return
			}

			// flush any buffered aggregation packets.
			flushBufferedNals()

			fullNALUSize := len(nalu)
			for len(nalu) > 0 {
				curentFUPayloadSize := len(nalu)
				if curentFUPayloadSize > maxFUPayloadSize {
					curentFUPayloadSize = maxFUPayloadSize
				}

				out := make([]byte, fuPacketHeaderSize+curentFUPayloadSize)

				// write the payload header
				binary.BigEndian.PutUint16(out[0:2], uint16(naluHeader))
				out[0] = (out[0] & 0b10000001) | h265NaluFragmentationUnitType<<1

				// write the fragment header
				out[2] = byte(H265FragmentationUnitHeader(naluHeader.Type()))
				if len(nalu) == fullNALUSize {
					// Set start bit
					out[2] |= 1 << 7
				} else if len(nalu)-curentFUPayloadSize == 0 {
					// Set end bit
					out[2] |= 1 << 6
				}

				if p.AddDONL {
					// write the DONL header
					binary.BigEndian.PutUint16(out[3:5], p.donl)

					p.donl++

					// copy the fragment payload
					copy(out[5:], nalu[0:curentFUPayloadSize])
				} else {
					// copy the fragment payload
					copy(out[3:], nalu[0:curentFUPayloadSize])
				}

				// append the fragment to the payload
				payloads = append(payloads, out)

				// advance the nalu data pointer
				nalu = nalu[curentFUPayloadSize:]
			}
		}
	})

	flushBufferedNals()

	return payloads
}
