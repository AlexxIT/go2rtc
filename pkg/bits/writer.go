package bits

type Writer struct {
	buf  []byte // total buf
	byte *byte  // pointer to current byte
	bits byte   // bits left in byte
}

func NewWriter(buf []byte) *Writer {
	return &Writer{buf: buf}
}

//goland:noinspection GoStandardMethods
func (w *Writer) WriteByte(b byte) {
	if w.bits != 0 {
		w.WriteBits8(b, 8)
	}

	w.buf = append(w.buf, b)
}

func (w *Writer) WriteBit(b byte) {
	if w.bits == 0 {
		w.buf = append(w.buf, 0)
		w.byte = &w.buf[len(w.buf)-1]
		w.bits = 7
	} else {
		w.bits--
	}

	*w.byte |= (b & 1) << w.bits
}

func (w *Writer) WriteBits(v uint32, n byte) {
	for i := n - 1; i != 255; i-- {
		w.WriteBit(byte(v>>i) & 0b1)
	}
}

func (w *Writer) WriteBits16(v uint16, n byte) {
	for i := n - 1; i != 255; i-- {
		w.WriteBit(byte(v>>i) & 0b1)
	}
}

func (w *Writer) WriteBits8(v, n byte) {
	for i := n - 1; i != 255; i-- {
		w.WriteBit((v >> i) & 0b1)
	}
}

func (w *Writer) WriteAllBits(bit, n byte) {
	for i := byte(0); i < n; i++ {
		w.WriteBit(bit)
	}
}

func (w *Writer) WriteUint16(v uint16) {
	if w.bits != 0 {
		w.WriteBits16(v, 16)
	}

	w.buf = append(w.buf, byte(v>>8), byte(v))
}

func (w *Writer) WriteBytes(bytes ...byte) {
	if w.bits != 0 {
		for _, b := range bytes {
			w.WriteByte(b)
		}
	}

	w.buf = append(w.buf, bytes...)
}

func (w *Writer) Bytes() []byte {
	return w.buf
}

func (w *Writer) Len() int {
	return len(w.buf)
}

func (w *Writer) Reset() {
	w.buf = w.buf[:0]
	w.bits = 0
}
