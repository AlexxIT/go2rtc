// Package av1 implements AV1 OBU parsing, keyframe detection, and RTP depayloading.
//
// References:
//   - AV1 Bitstream & Decoding Process: https://aomediacodec.github.io/av1-spec/
//   - AV1 RTP Payload Format (RFC 9583): https://www.rfc-editor.org/rfc/rfc9583
//   - AV1 Codec ISO Media File Format Binding: https://aomediacodec.github.io/av1-isobmff/
package av1

// OBU types from AV1 spec Section 6.2.2
const (
	OBUTypeSequenceHeader       = 1
	OBUTypeTemporalDelimiter    = 2
	OBUTypeFrameHeader          = 3
	OBUTypeTileGroup            = 4
	OBUTypeMetadata             = 5
	OBUTypeFrame                = 6
	OBUTypeRedundantFrameHeader = 7
	OBUTypeTileList             = 8
	OBUTypePadding              = 15
)

// Frame types from AV1 spec Section 6.8.2
const (
	FrameTypeKey   = 0
	FrameTypeInter = 1
)

// OBUType returns the OBU type from the first byte of an OBU header.
func OBUType(header byte) byte {
	return (header >> 3) & 0x0F
}

// OBUHasExtension returns true if the extension flag is set.
func OBUHasExtension(header byte) bool {
	return header&0x04 != 0
}

// OBUHasSize returns true if the has_size_field flag is set.
func OBUHasSize(header byte) bool {
	return header&0x02 != 0
}

// ReadLEB128 reads a LEB128 (Little Endian Base 128) encoded unsigned integer.
// Returns the value and the number of bytes consumed.
func ReadLEB128(data []byte) (value uint32, n int) {
	for i := 0; i < len(data) && i < 8; i++ {
		value |= uint32(data[i]&0x7F) << (i * 7)
		n = i + 1
		if data[i]&0x80 == 0 {
			return
		}
	}
	return
}

// WriteLEB128 encodes a value as LEB128 and returns the bytes.
func WriteLEB128(value uint32) []byte {
	if value == 0 {
		return []byte{0}
	}
	var buf []byte
	for value > 0 {
		b := byte(value & 0x7F)
		value >>= 7
		if value > 0 {
			b |= 0x80
		}
		buf = append(buf, b)
	}
	return buf
}

// OBUHeaderSize returns the total header size (1 for basic, 2 with extension).
func OBUHeaderSize(header byte) int {
	if OBUHasExtension(header) {
		return 2
	}
	return 1
}

// ParseOBUs iterates over OBUs in an AV1 bitstream (with size fields).
// Calls fn with (obuType, obuData including header) for each OBU.
// Returns false from fn to stop iteration.
func ParseOBUs(data []byte, fn func(obuType byte, obu []byte) bool) {
	for len(data) > 0 {
		header := data[0]
		obuType := OBUType(header)
		hdrSize := OBUHeaderSize(header)

		if len(data) < hdrSize {
			return
		}

		if !OBUHasSize(header) {
			// OBU without size field - rest of data is this OBU
			if !fn(obuType, data) {
				return
			}
			return
		}

		// read size after header
		if len(data) < hdrSize+1 {
			return
		}

		size, sizeLen := ReadLEB128(data[hdrSize:])
		totalSize := hdrSize + sizeLen + int(size)

		if totalSize > len(data) {
			return
		}

		if !fn(obuType, data[:totalSize]) {
			return
		}

		data = data[totalSize:]
	}
}

// IsKeyframe checks if the AV1 payload (sequence of OBUs in AVCC/MP4 format) contains a keyframe.
// In AV1, a keyframe is indicated by frame_type == KEY_FRAME in the frame header.
func IsKeyframe(payload []byte) bool {
	keyframe := false

	ParseOBUs(payload, func(obuType byte, obu []byte) bool {
		switch obuType {
		case OBUTypeSequenceHeader:
			// Sequence header often precedes keyframes
			keyframe = true
		case OBUTypeFrame, OBUTypeFrameHeader:
			hdrSize := OBUHeaderSize(obu[0])
			if !OBUHasSize(obu[0]) {
				return false
			}
			_, sizeLen := ReadLEB128(obu[hdrSize:])
			frameData := obu[hdrSize+sizeLen:]
			if len(frameData) > 0 {
				// frame_type is in the first few bits of the uncompressed header
				// show_existing_frame (1 bit) - if set, no frame data follows
				showExisting := (frameData[0] >> 7) & 1
				if showExisting == 1 {
					return true // not a real frame
				}
				// frame_type (2 bits after show_existing_frame)
				frameType := (frameData[0] >> 5) & 0x03
				if frameType == FrameTypeKey {
					keyframe = true
					return false // found keyframe, stop
				}
				keyframe = false // found a non-key frame
				return false
			}
		case OBUTypeTemporalDelimiter, OBUTypePadding:
			// skip
		default:
			// other OBU types
		}
		return true
	})

	return keyframe
}

// SequenceHeader extracts the raw sequence header OBU bytes from a payload.
// Returns nil if no sequence header is found.
func SequenceHeader(payload []byte) []byte {
	var seqHdr []byte
	ParseOBUs(payload, func(obuType byte, obu []byte) bool {
		if obuType == OBUTypeSequenceHeader {
			seqHdr = make([]byte, len(obu))
			copy(seqHdr, obu)
			return false
		}
		return true
	})
	return seqHdr
}
