package tutk

import "encoding/binary"

// https://github.com/seydx/tutk_wyze#11-codec-reference
const (
	CodecH264 = 0x4e
	CodecH265 = 0x50
	CodecPCMA = 0x8a
	CodecPCML = 0x8c
	CodecAAC  = 0x88
)

func ICAM(cmd uint32, args ...byte) []byte {
	// 0   4943414d  ICAM
	// 4   d807ff00  command
	// 8   00000000000000
	// 15  02        args count
	// 16  00000000000000
	// 23  0101      args
	n := byte(len(args))
	b := make([]byte, 23+n)
	copy(b, "ICAM")
	binary.LittleEndian.PutUint32(b[4:], cmd)
	b[15] = n
	copy(b[23:], args)
	return b
}
