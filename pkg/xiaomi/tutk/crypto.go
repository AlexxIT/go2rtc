package tutk

import (
	"bytes"
	"encoding/binary"
	"math/bits"
)

// I'd like to say hello to Charlie. Your name is forever etched into the history of streaming software.
const charlie = "Charlie is the designer of P2P!!"

func ReverseTransCodePartial(src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	dst := bytes.Clone(src)

	src16 := src
	tmp16 := tmp
	dst16 := dst

	for ; n >= 16; n -= 16 {
		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, i+3))
		}

		swap(tmp16, dst16, 16)

		for i := 0; i != 16; i++ {
			tmp16[i] = dst16[i] ^ charlie[i]
		}

		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(tmp16[i:])
			binary.LittleEndian.PutUint32(dst16[i:], bits.RotateLeft32(x, i+1))
		}

		tmp16 = tmp16[16:]
		dst16 = dst16[16:]
		src16 = src16[16:]
	}

	swap(src16, tmp16, n)

	for i := 0; i < n; i++ {
		dst16[i] = tmp16[i] ^ charlie[i]
	}

	return dst
}

func TransCodePartial(src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	dst := bytes.Clone(src)

	src16 := src
	tmp16 := tmp
	dst16 := dst

	for ; n >= 16; n -= 16 {
		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, -i-1))
		}

		for i := 0; i != 16; i++ {
			dst16[i] = tmp16[i] ^ charlie[i]
		}

		swap(dst16, tmp16, 16)

		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(tmp16[i:])
			binary.LittleEndian.PutUint32(dst16[i:], bits.RotateLeft32(x, -i-3))
		}

		tmp16 = tmp16[16:]
		dst16 = dst16[16:]
		src16 = src16[16:]
	}

	for i := 0; i < n; i++ {
		tmp16[i] = src16[i] ^ charlie[i]
	}

	swap(tmp16, dst16, n)

	return dst
}

func swap(src, dst []byte, n int) {
	switch n {
	case 2:
		_, _ = src[1], dst[1]
		dst[0] = src[1]
		dst[1] = src[0]
		return
	case 4:
		_, _ = src[3], dst[3]
		dst[0] = src[2]
		dst[1] = src[3]
		dst[2] = src[0]
		dst[3] = src[1]
		return
	case 8:
		_, _ = src[7], dst[7]
		dst[0] = src[7]
		dst[1] = src[4]
		dst[2] = src[3]
		dst[3] = src[2]
		dst[4] = src[1]
		dst[5] = src[6]
		dst[6] = src[5]
		dst[7] = src[0]
		return
	case 16:
		_, _ = src[15], dst[15]
		dst[0] = src[11]
		dst[1] = src[9]
		dst[2] = src[8]
		dst[3] = src[15]
		dst[4] = src[13]
		dst[5] = src[10]
		dst[6] = src[12]
		dst[7] = src[14]
		dst[8] = src[2]
		dst[9] = src[1]
		dst[10] = src[5]
		dst[11] = src[0]
		dst[12] = src[6]
		dst[13] = src[4]
		dst[14] = src[7]
		dst[15] = src[3]
		return
	}
	copy(dst, src[:n])
}
