package reolink

import "encoding/binary"

// IMA (DVI4) ADPCM, the only format Reolink cameras accept for talk

var imaIndex = [16]int{-1, -1, -1, -1, 2, 4, 6, 8, -1, -1, -1, -1, 2, 4, 6, 8}

var imaStep = [89]int{
	7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31, 34, 37, 41, 45, 50, 55, 60, 66, 73, 80, 88,
	97, 107, 118, 130, 143, 157, 173, 190, 209, 230, 253, 279, 307, 337, 371, 408, 449, 494, 544, 598, 658,
	724, 796, 876, 963, 1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066, 2272, 2499, 2749, 3024, 3327, 3660,
	4026, 4428, 4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635, 13899, 15289, 16818,
	18500, 20350, 22385, 24623, 27086, 29794, 32767,
}

type adpcmEncoder struct {
	predictor int
	index     int
}

func (e *adpcmEncoder) encodeSample(sample int16) byte {
	step := imaStep[e.index]
	diff := int(sample) - e.predictor

	var code int
	if diff < 0 {
		code = 8
		diff = -diff
	}

	delta := step >> 3
	for bit := 4; bit > 0; bit >>= 1 {
		if diff >= step {
			code |= bit
			diff -= step
			delta += step
		}
		step >>= 1
	}

	if code&8 != 0 {
		e.predictor -= delta
	} else {
		e.predictor += delta
	}
	e.predictor = min(max(e.predictor, -32768), 32767)
	e.index = min(max(e.index+imaIndex[code], 0), 88)

	return byte(code)
}

// encodeBlock returns a 4 byte preamble with the encoder state at the start of the block
// (predictor int16 LE, step index, zero) and two samples per byte, first sample in the high nibble
// as in RFC 3551 DVI4 (IMA WAV uses the low nibble first, which plays but sounds rough)
func (e *adpcmEncoder) encodeBlock(samples []int16) []byte {
	b := make([]byte, 4, 4+len(samples)/2)
	binary.LittleEndian.PutUint16(b, uint16(int16(e.predictor)))
	b[2] = byte(e.index)

	for i := 0; i+1 < len(samples); i += 2 {
		hi := e.encodeSample(samples[i])
		lo := e.encodeSample(samples[i+1])
		b = append(b, hi<<4|lo)
	}
	return b
}

// adpcmMagic is "01wb", the media header of ADPCM audio in Baichuan streams
var adpcmMagic = []byte{0x30, 0x31, 0x77, 0x62}

// appendFrame packs an ADPCM block the way the camera packs its own audio: magic, the length twice,
// an inner header 00 01 with the size of the packed samples, the block and zero padding to 8 bytes
func appendFrame(dst, block []byte) []byte {
	size := uint16(len(block) + 4)
	dst = append(dst, adpcmMagic...)
	dst = binary.LittleEndian.AppendUint16(dst, size)
	dst = binary.LittleEndian.AppendUint16(dst, size)
	dst = append(dst, 0x00, 0x01)
	dst = binary.LittleEndian.AppendUint16(dst, uint16(len(block)-4))
	dst = append(dst, block...)
	if n := len(block) % 8; n != 0 {
		dst = append(dst, make([]byte, 8-n)...)
	}
	return dst
}
