package h264

import (
	"bytes"
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func AnnexB2AVC(b []byte) []byte {
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

func AVCtoAnnexB(b []byte) []byte {
	b = bytes.Clone(b)
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

const forbiddenZeroBit = 0x80
const nalUnitType = 0x1F

// DecodeStream - find and return first AU in AVC format
// useful for processing live streams with unknown separator size
func DecodeStream(annexb []byte) ([]byte, int) {
	startPos := -1

	i := 0
	for {
		// search next separator
		if i = IndexFrom(annexb, []byte{0, 0, 1}, i); i < 0 {
			break
		}

		// move i to next AU
		if i += 3; i >= len(annexb) {
			break
		}

		// check if AU type valid
		octet := annexb[i]
		if octet&forbiddenZeroBit != 0 {
			continue
		}

		// 0 => AUD => SPS/IF/PF => AUD
		// 0 => SPS/PF => SPS/PF
		nalType := octet & nalUnitType
		if startPos >= 0 {
			switch nalType {
			case NALUTypeAUD, NALUTypeSPS, NALUTypePFrame:
				if annexb[i-4] == 0 {
					return DecodeAnnexB(annexb[startPos : i-4]), i - 4
				} else {
					return DecodeAnnexB(annexb[startPos : i-3]), i - 3
				}
			}
		} else {
			switch nalType {
			case NALUTypeSPS, NALUTypePFrame:
				if i >= 4 && annexb[i-4] == 0 {
					startPos = i - 4
				} else {
					startPos = i - 3
				}
			}
		}
	}

	return nil, 0
}

// DecodeAnnexB - convert AnnexB to AVC format
// support unknown separator size
func DecodeAnnexB(b []byte) []byte {
	if b[2] == 1 {
		// convert: 0 0 1 => 0 0 0 1
		b = append([]byte{0}, b...)
	}

	startPos := 0

	i := 4
	for {
		// search next separato
		if i = IndexFrom(b, []byte{0, 0, 1}, i); i < 0 {
			break
		}

		// move i to next AU
		if i += 3; i >= len(b) {
			break
		}

		// check if AU type valid
		octet := b[i]
		if octet&forbiddenZeroBit != 0 {
			continue
		}

		switch octet & nalUnitType {
		case NALUTypePFrame, NALUTypeIFrame, NALUTypeSPS, NALUTypePPS:
			if b[i-4] != 0 {
				// prefix: 0 0 1
				binary.BigEndian.PutUint32(b[startPos:], uint32(i-startPos-7))
				tmp := make([]byte, 0, len(b)+1)
				tmp = append(tmp, b[:i]...)
				tmp = append(tmp, 0)
				b = append(tmp, b[i:]...)
				startPos = i - 3
			} else {
				// prefix: 0 0 0 1
				binary.BigEndian.PutUint32(b[startPos:], uint32(i-startPos-8))
				startPos = i - 4
			}
		}
	}

	binary.BigEndian.PutUint32(b[startPos:], uint32(len(b)-startPos-4))
	return b
}

func IndexFrom(b []byte, sep []byte, from int) int {
	if from > 0 {
		if from < len(b) {
			if i := bytes.Index(b[from:], sep); i >= 0 {
				return from + i
			}
		}
		return -1
	}

	return bytes.Index(b, sep)
}

func EncodeAVC(nals ...[]byte) (avc []byte) {
	var i, n int

	for _, nal := range nals {
		if i = len(nal); i > 0 {
			n += 4 + i
		}
	}

	avc = make([]byte, n)

	n = 0
	for _, nal := range nals {
		if i = len(nal); i > 0 {
			binary.BigEndian.PutUint32(avc[n:], uint32(i))
			n += 4 + copy(avc[n+4:], nal)
		}
	}

	return
}

func RepairAVC(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	sps, pps := GetParameterSet(codec.FmtpLine)
	ps := EncodeAVC(sps, pps)

	return func(packet *rtp.Packet) {
		if NALUType(packet.Payload) == NALUTypeIFrame {
			packet.Payload = Join(ps, packet.Payload)
		}
		handler(packet)
	}
}

func SplitAVC(data []byte) [][]byte {
	var nals [][]byte
	for {
		// get AVC length
		size := int(binary.BigEndian.Uint32(data)) + 4

		// check if multiple items in one packet
		if size < len(data) {
			nals = append(nals, data[:size])
			data = data[size:]
		} else {
			nals = append(nals, data)
			break
		}
	}
	return nals
}

func Types(data []byte) []byte {
	var types []byte
	for {
		types = append(types, NALUType(data))

		size := 4 + int(binary.BigEndian.Uint32(data))
		if size < len(data) {
			data = data[size:]
		} else {
			break
		}
	}
	return types
}
