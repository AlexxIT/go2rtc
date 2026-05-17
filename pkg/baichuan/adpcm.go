// Package baichuan provides the protocol implementation for communicating with Baichuan cameras.
package baichuan

import (
	"encoding/binary"
	"fmt"
)

var imaIndexTable = []int{
	-1, -1, -1, -1, 2, 4, 6, 8,
	-1, -1, -1, -1, 2, 4, 6, 8,
}

var imaStepTable = []int{
	7, 8, 9, 10, 11, 12, 13, 14, 16, 17,
	19, 21, 23, 25, 28, 31, 34, 37, 41, 45,
	50, 55, 60, 66, 73, 80, 88, 97, 107, 118,
	130, 143, 157, 173, 190, 209, 230, 253, 279, 307,
	337, 371, 408, 449, 494, 544, 598, 658, 724, 796,
	876, 963, 1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066,
	2272, 2499, 2749, 3024, 3327, 3660, 4026, 4428, 4871, 5358,
	5894, 6484, 7132, 7845, 8630, 9493, 10442, 11487, 12635, 13899,
	15289, 16818, 18500, 20350, 22385, 24623, 27086, 29794, 32767,
}

// ADPCMDecoder provides state for decoding ADPCM audio streams.
type ADPCMDecoder struct {
	predicted int
	index     int
}

// ADPCMEncoder provides state for encoding PCM audio into IMA ADPCM blocks.
type ADPCMEncoder struct {
	predicted int
	index     int
}

// Decode decodes a chunk of ADPCM encoded audio into PCM samples.
func (d *ADPCMDecoder) Decode(data []byte) []int16 {
	if len(data) < 4 {
		return nil
	}

	d.predicted = int(int16(binary.LittleEndian.Uint16(data[0:2]))) //#nosec G115
	d.index = int(data[2])
	if d.index < 0 {
		d.index = 0
	}
	if d.index > 88 {
		d.index = 88
	}

	payload := data[4:]
	out := make([]int16, len(payload)*2)
	for i, b := range payload {
		// DVI4 packs the first sample in the high nibble and the second in the low nibble.
		nibbles := []byte{(b >> 4) & 0x0F, b & 0x0F}
		for j, nibble := range nibbles {
			step := imaStepTable[d.index]
			diff := step >> 3
			if (nibble & 1) != 0 {
				diff += step >> 2
			}
			if (nibble & 2) != 0 {
				diff += step >> 1
			}
			if (nibble & 4) != 0 {
				diff += step
			}

			if (nibble & 8) != 0 {
				d.predicted -= diff
			} else {
				d.predicted += diff
			}

			if d.predicted > 32767 {
				d.predicted = 32767
			}
			if d.predicted < -32768 {
				d.predicted = -32768
			}

			d.index += imaIndexTable[nibble]
			if d.index < 0 {
				d.index = 0
			}
			if d.index > 88 {
				d.index = 88
			}

			out[i*2+j] = int16(d.predicted) //#nosec G115
		}
	}
	return out
}

// EncodeBlock encodes one IMA ADPCM block.
// The returned block includes the 4-byte predictor header expected by Baichuan.
func (e *ADPCMEncoder) EncodeBlock(pcm []int16) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, nil
	}
	if len(pcm) < 2 {
		return nil, fmt.Errorf("adpcm block requires at least 2 samples, got %d", len(pcm))
	}
	if len(pcm)%2 != 0 {
		return nil, fmt.Errorf("adpcm block sample count must be even, got %d", len(pcm))
	}

	out := make([]byte, 4+len(pcm)/2)
	binary.LittleEndian.PutUint16(out[0:2], uint16(int16(e.predicted))) //#nosec G115
	out[2] = byte(e.index)                                              //#nosec G115
	out[3] = 0

	writePos := 4
	for i := 0; i < len(pcm); i += 2 {
		first := e.encodeNibble(int(pcm[i]))
		second := e.encodeNibble(int(pcm[i+1]))
		out[writePos] = (first << 4) | second
		writePos++
	}

	return out, nil
}

func (e *ADPCMEncoder) encodeNibble(sample int) byte {
	step := imaStepTable[e.index]
	diff := sample - e.predicted
	nibble := byte(0)

	if diff < 0 {
		nibble |= 8
		diff = -diff
	}

	delta := step >> 3
	if diff >= step {
		nibble |= 4
		diff -= step
		delta += step
	}
	if diff >= (step >> 1) {
		nibble |= 2
		diff -= step >> 1
		delta += step >> 1
	}
	if diff >= (step >> 2) {
		nibble |= 1
		delta += step >> 2
	}

	if (nibble & 8) != 0 {
		e.predicted -= delta
	} else {
		e.predicted += delta
	}

	if e.predicted > 32767 {
		e.predicted = 32767
	}
	if e.predicted < -32768 {
		e.predicted = -32768
	}

	e.index += imaIndexTable[nibble]
	if e.index < 0 {
		e.index = 0
	}
	if e.index > 88 {
		e.index = 88
	}

	return nibble
}
