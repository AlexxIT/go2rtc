package bits

type Writer struct {
	buf  []byte // total buf
	byte byte   // current byte
	bits byte   // bits left in byte
	len  int    // current len of buf
}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) WriteBit(b byte) {
	if w.bits == 0 {
		if w.len != 0 {
			w.buf = append(w.buf, w.byte)
		}

		w.byte = 0
		w.bits = 7
		w.len++
	} else {
		w.bits--
	}

	w.byte |= b << w.bits
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

func (w *Writer) Bytes() []byte {
	if w.bits == 0 {
		return w.buf
	}
	return append(w.buf, w.byte)
}
