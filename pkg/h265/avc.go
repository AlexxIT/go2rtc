package h265

import "github.com/AlexxIT/go2rtc/pkg/h264"

const forbiddenZeroBit = 0x80
const nalUnitType = 0x3F

// Deprecated: DecodeStream - find and return first AU in AVC format
// useful for processing live streams with unknown separator size
func DecodeStream(annexb []byte) ([]byte, int) {
	startPos := -1

	i := 0
	for {
		// search next separator
		if i = h264.IndexFrom(annexb, []byte{0, 0, 1}, i); i < 0 {
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

		nalType := (octet >> 1) & nalUnitType
		if startPos >= 0 {
			switch nalType {
			case NALUTypeVPS, NALUTypePFrame:
				if annexb[i-4] == 0 {
					return h264.DecodeAnnexB(annexb[startPos : i-4]), i - 4
				} else {
					return h264.DecodeAnnexB(annexb[startPos : i-3]), i - 3
				}
			}
		} else {
			switch nalType {
			case NALUTypeVPS, NALUTypePFrame:
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
