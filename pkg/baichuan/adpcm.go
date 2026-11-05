package baichuan

import (
	"encoding/binary"
	"fmt"
)

var imaIndex = [...]int8{
	-1, -1, -1, -1, 2, 4, 6, 8,
	-1, -1, -1, -1, 2, 4, 6, 8,
}

var imaStep = [...]int{
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

// ADPCMEncoder encodes the IMA ADPCM block variant used by Baichuan talkback.
type ADPCMEncoder struct {
	predictor int
	index     int
}

// DecodeADPCMBlock decodes the block variant produced by Baichuan cameras.
func DecodeADPCMBlock(block []byte) ([]int16, error) {
	if len(block) < 5 {
		return nil, fmt.Errorf("baichuan: ADPCM block too short: %d", len(block))
	}
	predictor := int(int16(binary.LittleEndian.Uint16(block)))
	index := int(block[2])
	if index >= len(imaStep) {
		return nil, fmt.Errorf("baichuan: invalid ADPCM index %d", index)
	}
	samples := make([]int16, (len(block)-4)*2)
	for i, value := range block[4:] {
		samples[i*2] = int16(decodeADPCM(value>>4, &predictor, &index))
		samples[i*2+1] = int16(decodeADPCM(value&0x0f, &predictor, &index))
	}
	return samples, nil
}

func decodeADPCM(nibble byte, predictor, index *int) int {
	step := imaStep[*index]
	delta := step >> 3
	if nibble&1 != 0 {
		delta += step >> 2
	}
	if nibble&2 != 0 {
		delta += step >> 1
	}
	if nibble&4 != 0 {
		delta += step
	}
	if nibble&8 != 0 {
		*predictor -= delta
	} else {
		*predictor += delta
	}
	*predictor = clamp(*predictor, -32768, 32767)
	*index = clamp(*index+int(imaIndex[nibble]), 0, len(imaStep)-1)
	return *predictor
}

func (e *ADPCMEncoder) EncodeBlock(samples []int16) ([]byte, error) {
	if len(samples) < 2 || len(samples)&1 != 0 {
		return nil, fmt.Errorf("baichuan: ADPCM block needs a positive even sample count, got %d", len(samples))
	}
	b := make([]byte, 4+len(samples)/2)
	binary.LittleEndian.PutUint16(b, uint16(int16(e.predictor)))
	b[2] = byte(e.index)
	for i := 0; i < len(samples); i += 2 {
		b[4+i/2] = e.encode(int(samples[i]))<<4 | e.encode(int(samples[i+1]))
	}
	return b, nil
}

func (e *ADPCMEncoder) encode(sample int) byte {
	step := imaStep[e.index]
	diff := sample - e.predictor
	var nibble byte
	if diff < 0 {
		nibble = 8
		diff = -diff
	}
	delta := step >> 3
	for mask, part := byte(4), step; mask != 0; mask, part = mask>>1, part>>1 {
		if diff >= part {
			nibble |= mask
			diff -= part
			delta += part
		}
	}
	if nibble&8 != 0 {
		e.predictor -= delta
	} else {
		e.predictor += delta
	}
	e.predictor = clamp(e.predictor, -32768, 32767)
	e.index = clamp(e.index+int(imaIndex[nibble]), 0, len(imaStep)-1)
	return nibble
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
