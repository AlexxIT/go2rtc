// Package annexb - universal for H264 and H265
package annexb

import (
	"bytes"
	"encoding/binary"
)

const StartCode = "\x00\x00\x00\x01"
const startAUD = StartCode + "\x09\xF0"
const startAUDstart = startAUD + StartCode

// EncodeToAVCC
// will change original slice data!
// safeAppend should be used if original slice has useful data after end (part of other slice)
//
// FFmpeg MPEG-TS: 00000001 AUD 00000001 SPS 00000001 PPS 000001 IFrame
// FFmpeg H264:    00000001 SPS 00000001 PPS 000001 IFrame 00000001 PFrame
func EncodeToAVCC(b []byte, safeAppend bool) []byte {
	const minSize = len(StartCode) + 1

	// 1. Check frist "start code"
	if len(b) < len(startAUDstart) || string(b[:len(StartCode)]) != StartCode {
		return nil
	}

	// 2. Skip Access unit delimiter (AUD) from FFmpeg
	if string(b[:len(startAUDstart)]) == startAUDstart {
		b = b[6:]
	}

	var start int

	for i, n := minSize, len(b)-minSize; i < n; {
		// 3. Check "start code" (first 2 bytes)
		if b[i] != 0 || b[i+1] != 0 {
			i++
			continue
		}

		// 4. Check "start code" (3 bytes size or 4 bytes size)
		if b[i+2] == 1 {
			if safeAppend {
				// protect original slice from "damage"
				b = bytes.Clone(b)
				safeAppend = false
			}

			// convert start code from 3 bytes to 4 bytes
			b = append(b, 0)
			copy(b[i+1:], b[i:])
			n++
		} else if b[i+2] != 0 || b[i+3] != 1 {
			i++
			continue
		}

		// 5. Set size for previous AU
		size := uint32(i - start - len(StartCode))
		binary.BigEndian.PutUint32(b[start:], size)

		start = i

		i += minSize
	}

	// 6. Set size for last AU
	size := uint32(len(b) - start - len(StartCode))
	binary.BigEndian.PutUint32(b[start:], size)

	return b
}

func DecodeAVCC(b []byte, safeClone bool) []byte {
	if safeClone {
		b = bytes.Clone(b)
	}
	for i := 0; i < len(b); {
		size := int(binary.BigEndian.Uint32(b[i:]))
		b[i] = 0
		b[i+1] = 0
		b[i+2] = 0
		b[i+3] = 1
		i += 4 + size
	}
	return b
}

// DecodeAVCCWithAUD - AUD doesn't important for FFmpeg, but important for Safari
func DecodeAVCCWithAUD(src []byte) []byte {
	dst := make([]byte, len(startAUD)+len(src))
	copy(dst, startAUD)
	copy(dst[len(startAUD):], src)
	DecodeAVCC(dst[len(startAUD):], false)
	return dst
}

const (
	h264PFrame = 1
	h264IFrame = 5
	h264SPS    = 7
	h264PPS    = 8

	h265VPS    = 32
	h265PFrame = 1
)

// IndexFrame - get new frame start position in the AnnexB stream
func IndexFrame(b []byte) int {
	if len(b) < len(startAUDstart) {
		return -1
	}

	for i := len(startAUDstart); ; {
		if di := bytes.Index(b[i:], []byte(StartCode)); di < 0 {
			break
		} else {
			i += di + 4 // move to NALU start
		}

		if i >= len(b) {
			break
		}

		h264Type := b[i] & 0b1_1111
		switch h264Type {
		case h264PFrame, h264SPS:
			return i - 4 // move to start code
		case h264IFrame, h264PPS:
			continue
		}

		h265Type := (b[i] >> 1) & 0b11_1111
		switch h265Type {
		case h265PFrame, h265VPS:
			return i - 4 // move to start code
		}
	}

	return -1
}
