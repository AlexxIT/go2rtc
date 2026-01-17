package tutk

import (
	"encoding/binary"
	"math/bits"
)

// I'd like to say hello to Charlie. Your name is forever etched into the history of streaming software.
const charlie = "Charlie is the designer of P2P!!"

func ReverseTransCodePartial(dst, src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	if len(dst) < n {
		dst = make([]byte, n)
	}

	src16 := src
	tmp16 := tmp
	dst16 := dst

	for ; n >= 16; n -= 16 {
		for i := 0; i != 16; i += 4 {
			x := binary.LittleEndian.Uint32(src16[i:])
			binary.LittleEndian.PutUint32(tmp16[i:], bits.RotateLeft32(x, i+3))
		}

		swap(dst16, tmp16, 16)

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

	swap(tmp16, src16, n)

	for i := 0; i < n; i++ {
		dst16[i] = tmp16[i] ^ charlie[i]
	}

	return dst
}

func TransCodePartial(dst, src []byte) []byte {
	n := len(src)
	tmp := make([]byte, n)
	if len(dst) < n {
		dst = make([]byte, n)
	}

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

		swap(tmp16, dst16, 16)

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

	swap(dst16, tmp16, n)

	return dst
}

func swap(dst, src []byte, n int) {
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

const delta = 0x9e3779b9

func XXTEADecrypt(dst, src, key []byte) {
	const n = int8(4) // support only 16 bytes src

	var w, k [n]uint32
	for i := int8(0); i < n; i++ {
		w[i] = binary.LittleEndian.Uint32(src)
		k[i] = binary.LittleEndian.Uint32(key)
		src = src[4:]
		key = key[4:]
	}

	rounds := 52/n + 6
	sum := uint32(rounds) * delta
	for ; rounds > 0; rounds-- {
		w0 := w[0]
		i2 := int8((sum >> 2) & 3)
		for i := n - 1; i >= 0; i-- {
			wi := w[(i-1)&3]
			ki := k[i^i2]
			t1 := (w0 ^ sum) + (wi ^ ki)
			t2 := (wi >> 5) ^ (w0 << 2)
			t3 := (w0 >> 3) ^ (wi << 4)
			w[i] -= t1 ^ (t2 + t3)
			w0 = w[i]
		}
		sum -= delta
	}

	for _, i := range w {
		binary.LittleEndian.PutUint32(dst, i)
		dst = dst[4:]
	}
}
