package bits

type Reader struct {
	buf  []byte // packets buffer
	byte byte
	bits byte
	pos  int
}

func NewReader(b []byte) *Reader {
	return &Reader{buf: b}
}

//goland:noinspection GoStandardMethods
func (r *Reader) ReadByte() byte {
	if r.bits == 0 {
		b := r.buf[r.pos]
		r.pos++
		return b
	}

	return r.ReadBits8(8)
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

func (r *Reader) SkipBits(n int) {
	for i := 0; i < n; i++ {
		if r.bits == 0 {
			r.byte = r.buf[r.pos]
			r.pos++
			r.bits = 7
		} else {
			r.bits--
		}
	}
}
