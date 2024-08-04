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
//
// FFmpeg MPEG-TS: 00000001 AUD 00000001 SPS 00000001 PPS 000001 IFrame
// FFmpeg H264:    00000001 SPS 00000001 PPS 000001 IFrame 00000001 PFrame
// Reolink:        000001 AUD 000001 VPS 00000001 SPS 00000001 PPS 00000001 IDR 00000001 IDR
func EncodeToAVCC(annexb []byte) (avc []byte) {
	var start int

	avc = make([]byte, 0, len(annexb)+4) // init memory with little overhead

	for i := 0; ; i++ {
		var offset int

		if i+3 < len(annexb) {
			// search next separator
			if annexb[i] == 0 && annexb[i+1] == 0 {
				if annexb[i+2] == 1 {
					offset = 3 // 00 00 01
				} else if annexb[i+2] == 0 && annexb[i+3] == 1 {
					offset = 4 // 00 00 00 01
				} else {
					continue
				}
			} else {
				continue
			}
		} else {
			i = len(annexb) // move i to data end
		}

		if start != 0 {
			size := uint32(i - start)
			avc = binary.BigEndian.AppendUint32(avc, size)
			avc = append(avc, annexb[start:i]...)
		}

		// sometimes FFmpeg put separator at the end
		if i += offset; i == len(annexb) {
			break
		}

		if isAUD(annexb[i]) {
			start = 0 // skip this NALU
		} else {
			start = i // save this position
		}
	}

	return
}

func isAUD(b byte) bool {
	const h264 = 9
	const h265 = 35 << 1
	return b&0b0001_1111 == h264 || b&0b0111_1110 == h265
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

func FixAnnexBInAVCC(b []byte) []byte {
	for i := 0; i < len(b); {
		if i+4 >= len(b) {
			break
		}

		size := bytes.Index(b[i+4:], []byte{0, 0, 0, 1})
		if size < 0 {
			size = len(b) - (i + 4)
		}

		binary.BigEndian.PutUint32(b[i:], uint32(size))

		i += size + 4
	}

	return b
}
