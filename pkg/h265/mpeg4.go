// Package h265 - MPEG4 format related functions
package h265

import "encoding/binary"

func EncodeConfig(vps, sps, pps []byte) []byte {
	vpsSize := uint16(len(vps))
	spsSize := uint16(len(sps))
	ppsSize := uint16(len(pps))

	buf := make([]byte, 23+5+vpsSize+5+spsSize+5+ppsSize)

	buf[0] = 1
	copy(buf[1:], sps[3:6]) // profile
	buf[21] = 3             // ?
	buf[22] = 3             // ?

	b := buf[23:]
	_ = b[5]
	b[0] = (vps[0] >> 1) & 0x3F
	binary.BigEndian.PutUint16(b[1:], 1) // VPS count
	binary.BigEndian.PutUint16(b[3:], vpsSize)
	copy(b[5:], vps)

	b = buf[23+5+vpsSize:]
	_ = b[5]
	b[0] = (sps[0] >> 1) & 0x3F
	binary.BigEndian.PutUint16(b[1:], 1) // SPS count
	binary.BigEndian.PutUint16(b[3:], spsSize)
	copy(b[5:], sps)

	b = buf[23+5+vpsSize+5+spsSize:]
	_ = b[5]
	b[0] = (pps[0] >> 1) & 0x3F
	binary.BigEndian.PutUint16(b[1:], 1) // PPS count
	binary.BigEndian.PutUint16(b[3:], ppsSize)
	copy(b[5:], pps)

	return buf
}
