package golomb

import "bytes"

type Reader struct {
	r     *bytes.Reader
	b     byte
	shift byte
}

func NewReader(b []byte) *Reader {
	return &Reader{
		r: bytes.NewReader(b),
	}
}

func (g *Reader) ReadBit() (b byte, err error) {
	if g.shift == 0 {
		if g.b, err = g.r.ReadByte(); err != nil {
			return 0, err
		}
		g.shift = 7
	} else {
		g.shift--
	}
	b = (g.b >> g.shift) & 0b1
	return
}

func (g *Reader) ReadBits(n byte) (res uint, err error) {
	var b byte
	for i := n - 1; i != 255; i-- {
		if b, err = g.ReadBit(); err != nil {
			return
		}
		res |= uint(b) << i
	}
	return
}

func (g *Reader) ReadUEGolomb() (res uint, err error) {
	var b uint
	var i byte
	for i = 0; i < 32; i++ {
		if b, err = g.ReadBits(1); err != nil {
			return
		}
		if b != 0 {
			break
		}
	}
	if res, err = g.ReadBits(i); err != nil {
		return
	}
	res += (1 << i) - 1
	return
}

func (g *Reader) ReadSEGolomb() (res int, err error) {
	var b uint
	if b, err = g.ReadUEGolomb(); err != nil {
		return
	}
	if b%2 == 0 {
		res = -int(b >> 1)
	} else {
		res = int(b>>1)
	}
	return
}

func (g *Reader) ReadByte() (byte, error) {
	return g.r.ReadByte()
}

func (g *Reader) End() bool {
	// if only one bit in next byte left
	if g.shift == 0 && g.r.Len() == 1 {
		b, _ := g.r.ReadByte()
		_ = g.r.UnreadByte()
		return b == 0x80
	}
	if g.r.Len() == 0 {
		//panic("not implemented")
	}
	return false
}
