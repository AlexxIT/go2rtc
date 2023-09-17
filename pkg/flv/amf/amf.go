package amf

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	TypeNumber byte = iota
	TypeBoolean
	TypeString
	TypeObject
	TypeNull      = 5
	TypeEcmaArray = 8
	TypeObjectEnd = 9
)

// AMF spec: http://download.macromedia.com/pub/labs/amf/amf0_spec_121207.pdf
type AMF struct {
	buf []byte
	pos int
}

var ErrRead = errors.New("amf: read error")

func NewReader(b []byte) *AMF {
	return &AMF{buf: b}
}

func (a *AMF) ReadItems() ([]any, error) {
	var items []any
	for a.pos < len(a.buf) {
		v, err := a.ReadItem()
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
	return items, nil
}

func (a *AMF) ReadItem() (any, error) {
	dataType, err := a.ReadByte()
	if err != nil {
		return nil, err
	}

	switch dataType {
	case TypeNumber:
		return a.ReadNumber()

	case TypeBoolean:
		b, err := a.ReadByte()
		return b != 0, err

	case TypeString:
		return a.ReadString()

	case TypeObject:
		return a.ReadObject()

	case TypeEcmaArray:
		return a.ReadEcmaArray()

	case TypeNull:
		return nil, nil

	case TypeObjectEnd:
		return nil, nil
	}

	return nil, ErrRead
}

func (a *AMF) ReadByte() (byte, error) {
	if a.pos >= len(a.buf) {
		return 0, ErrRead
	}

	v := a.buf[a.pos]
	a.pos++
	return v, nil
}

func (a *AMF) ReadNumber() (float64, error) {
	if a.pos+8 > len(a.buf) {
		return 0, ErrRead
	}

	v := binary.BigEndian.Uint64(a.buf[a.pos : a.pos+8])
	a.pos += 8
	return math.Float64frombits(v), nil
}

func (a *AMF) ReadString() (string, error) {
	if a.pos+2 > len(a.buf) {
		return "", ErrRead
	}

	size := int(binary.BigEndian.Uint16(a.buf[a.pos:]))
	a.pos += 2

	if a.pos+size > len(a.buf) {
		return "", ErrRead
	}

	s := string(a.buf[a.pos : a.pos+size])
	a.pos += size

	return s, nil
}

func (a *AMF) ReadObject() (map[string]any, error) {
	obj := make(map[string]any)

	for {
		k, err := a.ReadString()
		if err != nil {
			return nil, err
		}

		v, err := a.ReadItem()
		if err != nil {
			return nil, err
		}

		if k == "" {
			break
		}

		obj[k] = v
	}

	return obj, nil
}

func (a *AMF) ReadEcmaArray() (map[string]any, error) {
	if a.pos+4 > len(a.buf) {
		return nil, ErrRead
	}
	a.pos += 4 // skip size

	return a.ReadObject()
}

func NewWriter() *AMF {
	return &AMF{}
}

func (a *AMF) Bytes() []byte {
	return a.buf
}

func (a *AMF) WriteNumber(n float64) {
	b := math.Float64bits(n)
	a.buf = append(
		a.buf, TypeNumber,
		byte(b>>56), byte(b>>48), byte(b>>40), byte(b>>32),
		byte(b>>24), byte(b>>16), byte(b>>8), byte(b),
	)
}

func (a *AMF) WriteBool(b bool) {
	if b {
		a.buf = append(a.buf, TypeBoolean, 1)
	} else {
		a.buf = append(a.buf, TypeBoolean, 0)
	}
}

func (a *AMF) WriteString(s string) {
	n := len(s)
	a.buf = append(a.buf, TypeString, byte(n>>8), byte(n))
	a.buf = append(a.buf, s...)
}

func (a *AMF) WriteObject(obj map[string]any) {
	a.buf = append(a.buf, TypeObject)
	a.writeKV(obj)
	a.buf = append(a.buf, 0, 0, TypeObjectEnd)
}

func (a *AMF) WriteEcmaArray(obj map[string]any) {
	n := len(obj)
	a.buf = append(a.buf, TypeEcmaArray, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	a.writeKV(obj)
	a.buf = append(a.buf, 0, 0, TypeObjectEnd)
}

func (a *AMF) writeKV(obj map[string]any) {
	for k, v := range obj {
		n := len(k)
		a.buf = append(a.buf, byte(n>>8), byte(n))
		a.buf = append(a.buf, k...)

		switch v := v.(type) {
		case string:
			a.WriteString(v)
		case int:
			a.WriteNumber(float64(v))
		case uint16:
			a.WriteNumber(float64(v))
		case uint32:
			a.WriteNumber(float64(v))
		case float64:
			a.WriteNumber(v)
		case bool:
			a.WriteBool(v)
		default:
			panic(v)
		}
	}
}

func (a *AMF) WriteNull() {
	a.buf = append(a.buf, TypeNull)
}

func EncodeItems(items ...any) []byte {
	a := &AMF{}
	for _, item := range items {
		switch v := item.(type) {
		case float64:
			a.WriteNumber(v)
		case int:
			a.WriteNumber(float64(v))
		case string:
			a.WriteString(v)
		case map[string]any:
			a.WriteObject(v)
		case nil:
			a.WriteNull()
		default:
			panic(v)
		}
	}
	return a.Bytes()
}
