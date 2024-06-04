package bits

type Reader struct {
	EOF bool // if end of buffer raised during reading

	buf  []byte // total buf
	byte byte   // current byte
	bits byte   // bits left in byte
	pos  int    // current pos in buf
}

func NewReader(b []byte) *Reader {
	return &Reader{buf: b}
}

//goland:noinspection GoStandardMethods
func (r *Reader) ReadByte() byte {
	if r.bits != 0 {
		return r.ReadBits8(8)
	}

	if r.pos >= len(r.buf) {
		r.EOF = true
		return 0
	}

	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *Reader) ReadUint16() uint16 {
	if r.bits != 0 {
		return r.ReadBits16(16)
	}
	return uint16(r.ReadByte())<<8 | uint16(r.ReadByte())
}

func (r *Reader) ReadUint24() uint32 {
	if r.bits != 0 {
		return r.ReadBits(24)
	}
	return uint32(r.ReadByte())<<16 | uint32(r.ReadByte())<<8 | uint32(r.ReadByte())
}

func (r *Reader) ReadUint32() uint32 {
	if r.bits != 0 {
		return r.ReadBits(32)
	}
	return uint32(r.ReadByte())<<24 | uint32(r.ReadByte())<<16 | uint32(r.ReadByte())<<8 | uint32(r.ReadByte())
}

func (r *Reader) ReadBit() byte {
	if r.bits == 0 {
		r.byte = r.ReadByte()
		r.bits = 7
	} else {
		r.bits--
	}

	return (r.byte >> r.bits) & 0b1
}

func (r *Reader) ReadBits(n byte) (res uint32) {
	for i := n - 1; i != 255; i-- {
		res |= uint32(r.ReadBit()) << i
	}
	return
}

func (r *Reader) ReadBits8(n byte) (res uint8) {
	for i := n - 1; i != 255; i-- {
		res |= r.ReadBit() << i
	}
	return
}

func (r *Reader) ReadBits16(n byte) (res uint16) {
	for i := n - 1; i != 255; i-- {
		res |= uint16(r.ReadBit()) << i
	}
	return
}

func (r *Reader) ReadBits64(n byte) (res uint64) {
	for i := n - 1; i != 255; i-- {
		res |= uint64(r.ReadBit()) << i
	}
	return
}

func (r *Reader) ReadBytes(n int) (b []byte) {
	if r.bits == 0 {
		if r.pos+n > len(r.buf) {
			r.EOF = true
			return nil
		}

		b = r.buf[r.pos : r.pos+n]
		r.pos += n
	} else {
		b = make([]byte, n)
		for i := 0; i < n; i++ {
			b[i] = r.ReadByte()
		}
	}

	return
}

// ReadUEGolomb - ReadExponentialGolomb (unsigned)
func (r *Reader) ReadUEGolomb() uint32 {
	var size byte
	for size = 0; size < 32; size++ {
		if b := r.ReadBit(); b != 0 || r.EOF {
			break
		}
	}
	return r.ReadBits(size) + (1 << size) - 1
}

// ReadSEGolomb - ReadSignedExponentialGolomb
func (r *Reader) ReadSEGolomb() int32 {
	if b := r.ReadUEGolomb(); b%2 == 0 {
		return -int32(b >> 1)
	} else {
		return int32(b >> 1)
	}
}

func (r *Reader) Left() []byte {
	return r.buf[r.pos:]
}

func (r *Reader) Pos() (int, byte) {
	return r.pos - 1, r.bits
}
