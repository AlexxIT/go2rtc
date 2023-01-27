package iso

import (
	"encoding/binary"
	"math"
)

type Movie struct {
	b     []byte
	start []int
}

func NewMovie(size int) *Movie {
	return &Movie{b: make([]byte, 0, size)}
}

func (m *Movie) Bytes() []byte {
	return m.b
}

func (m *Movie) StartAtom(name string) {
	m.start = append(m.start, len(m.b))
	m.b = append(m.b, 0, 0, 0, 0)
	m.b = append(m.b, name...)
}

func (m *Movie) EndAtom() {
	n := len(m.start) - 1

	i := m.start[n]
	size := uint32(len(m.b) - i)
	binary.BigEndian.PutUint32(m.b[i:], size)

	m.start = m.start[:n]
}

func (m *Movie) Write(b []byte) {
	m.b = append(m.b, b...)
}

func (m *Movie) WriteBytes(b ...byte) {
	m.b = append(m.b, b...)
}

func (m *Movie) WriteString(s string) {
	m.b = append(m.b, s...)
}

func (m *Movie) Skip(n int) {
	m.b = append(m.b, make([]byte, n)...)
}

func (m *Movie) WriteUint16(v uint16) {
	m.b = append(m.b, byte(v>>8), byte(v))
}

func (m *Movie) WriteUint24(v uint32) {
	m.b = append(m.b, byte(v>>16), byte(v>>8), byte(v))
}

func (m *Movie) WriteUint32(v uint32) {
	m.b = append(m.b, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (m *Movie) WriteUint64(v uint64) {
	m.b = append(m.b, byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32), byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func (m *Movie) WriteFloat16(f float64) {
	i, f := math.Modf(f)
	f *= 256
	m.b = append(m.b, byte(i), byte(f))
}

func (m *Movie) WriteFloat32(f float64) {
	i, f := math.Modf(f)
	f *= 65536
	m.b = append(m.b, byte(uint16(i)>>8), byte(i), byte(uint16(f)>>8), byte(f))
}

func (m *Movie) WriteMatrix() {
	m.WriteUint32(0x00010000)
	m.Skip(4)
	m.Skip(4)
	m.Skip(4)
	m.WriteUint32(0x00010000)
	m.Skip(4)
	m.Skip(4)
	m.Skip(4)
	m.WriteUint32(0x40000000)
}
