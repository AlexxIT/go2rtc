// Package av1 implements AV1 OBU parsing, keyframe detection, and RTP depayloading.
//
// References:
//   - AV1 Bitstream & Decoding Process: https://aomediacodec.github.io/av1-spec/
//   - AV1 RTP Payload Format (RFC 9583): https://www.rfc-editor.org/rfc/rfc9583
//   - AV1 Codec ISO Media File Format Binding: https://aomediacodec.github.io/av1-isobmff/
package av1

// OBU types from AV1 spec Section 6.2.2
const (
	OBUTypeSequenceHeader    = 1
	OBUTypeTemporalDelimiter = 2
	OBUTypeFrameHeader       = 3
	OBUTypeTileGroup         = 4
	OBUTypeFrame             = 6
)

// Frame type from AV1 spec Section 6.8.2
const FrameTypeKey = 0

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

// MaxSequenceHeaderSize bounds what SequenceHeader will hand on. Real ones are
// 13 to 20 bytes; the cap only keeps a padded OBU out of the fmtp line.
const MaxSequenceHeaderSize = 256

// ReadLEB128 reads a LEB128 (Little Endian Base 128) encoded unsigned integer.
// Returns the value and the number of bytes consumed, or n = 0 if the encoding
// is not terminated or does not fit in 32 bits, so callers can reject it.
func ReadLEB128(data []byte) (value uint32, n int) {
	for i := 0; i < len(data) && i < 8; i++ {
		b := data[i]
		// the 5th byte carries only 4 usable bits, the rest would overflow
		if i == 4 && b&0x7F > 0x0F {
			return 0, 0 // more than 32 bits
		}
		if i >= 5 && b&0x7F != 0 {
			return 0, 0 // more than 32 bits
		}
		value |= uint32(b&0x7F) << (i * 7)
		if b&0x80 == 0 {
			return value, i + 1
		}
	}
	return 0, 0 // no terminating byte
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

// obuPayload returns the OBU payload without the OBU header and the optional
// size field. Returns nil if the OBU is truncated.
func obuPayload(obu []byte) []byte {
	if len(obu) == 0 {
		return nil
	}

	n := OBUHeaderSize(obu[0])
	if len(obu) < n {
		return nil
	}

	if !OBUHasSize(obu[0]) {
		// the last OBU of a temporal unit may omit the size field
		return obu[n:]
	}

	size, sizeLen := ReadLEB128(obu[n:])
	if sizeLen == 0 || size > uint32(len(obu)) || len(obu) < n+sizeLen+int(size) {
		return nil
	}

	return obu[n+sizeLen : n+sizeLen+int(size)]
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
		if sizeLen == 0 {
			return // invalid size field, the rest can't be trusted
		}

		// compared before the conversion, because int is 32 bits on the 386,
		// arm and mips builds, where a large size would go negative
		if uint64(hdrSize)+uint64(sizeLen)+uint64(size) > uint64(len(data)) {
			return
		}

		totalSize := hdrSize + sizeLen + int(size)

		if !fn(obuType, data[:totalSize]) {
			return
		}

		data = data[totalSize:]
	}
}

// IsKeyframe checks if the AV1 temporal unit (sequence of OBUs in MP4 low
// overhead format) contains a keyframe, indicated by frame_type == KEY_FRAME in
// the uncompressed header. Decided by the first frame header in the unit.
func IsKeyframe(payload []byte) bool {
	var keyframe, reducedStill bool

	ParseOBUs(payload, func(obuType byte, obu []byte) bool {
		switch obuType {
		case OBUTypeSequenceHeader:
			// with reduced_still_picture_header every frame is a keyframe and
			// the uncompressed header carries no frame_type bits
			if data := obuPayload(obu); len(data) > 0 {
				reducedStill = data[0]&0x08 != 0 // after seq_profile(3) + still_picture(1)
			}

		case OBUTypeFrame, OBUTypeFrameHeader:
			if reducedStill {
				keyframe = true
				return false
			}

			data := obuPayload(obu)
			if len(data) == 0 {
				return true
			}
			if data[0]&0x80 != 0 {
				return true // show_existing_frame, this OBU has no frame_type
			}

			keyframe = (data[0]>>5)&0x03 == FrameTypeKey
			return false // the first real frame header decides
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
		if obuType != OBUTypeSequenceHeader {
			return true
		}
		// Without a size field the OBU runs to the end of the temporal unit,
		// which would put the whole keyframe into the av1C box and the fmtp
		// line. The sequence header is never the last OBU of a unit.
		if !OBUHasSize(obu[0]) {
			return false
		}
		// The OBU is copied into the fmtp line, the av1C box of every init
		// segment and every RTSP SDP answer, so a padded one would be served
		// back over and over. A real sequence header is a few dozen bytes.
		if len(obu) > MaxSequenceHeaderSize {
			return false
		}
		seqHdr = make([]byte, len(obu))
		copy(seqHdr, obu)
		return false
	})
	return seqHdr
}
