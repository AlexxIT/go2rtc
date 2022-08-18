package golomb

import "math/bits"

type Writer struct {
	buf   []byte
	b     byte // last byte
	i     int  // last byte index
	shift byte
}

func NewWriter() *Writer {
	return &Writer{i: -1}
}

func (g *Writer) WriteBit(b byte) {
	if g.shift == 0 {
		g.buf = append(g.buf, 0)
		g.b = 0
		g.i++
		g.shift = 7
	} else {
		g.shift--
	}
	g.b |= b << g.shift
	g.buf[g.i] = g.b
}

func (g *Writer) WriteBits(b, n byte) {
	for i := n - 1; i != 255; i-- {
		g.WriteBit((b >> i) & 0b1)
	}
}

func (g *Writer) WriteByte(b byte) {
	g.buf = append(g.buf, b)
	g.i++
}

func (g *Writer) WriteUEGolomb(b byte) {
	b++
	n := uint8(bits.Len8(b))*2 - 1
	g.WriteBits(b, n)
}

func (g *Writer) WriteSEGolomb(b int8) {
	if b > 0 {
		g.WriteUEGolomb(byte(b)*2 - 1)
	} else {
		g.WriteUEGolomb(byte(-b) * 2)
	}
}

func (g *Writer) Bytes() []byte {
	return g.buf
}
