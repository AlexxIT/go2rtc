package httpflv

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
	TypeEcmaArray = 8
	TypeObjectEnd = 9
)

var Err = errors.New("amf0 read error")

// AMF0 spec: http://download.macromedia.com/pub/labs/amf/amf0_spec_121207.pdf
type AMF0 struct {
	buf []byte
	pos int
}

func NewReader(b []byte) *AMF0 {
	return &AMF0{buf: b}
}

func (a *AMF0) ReadMetaData() map[string]interface{} {
	if b, _ := a.ReadByte(); b != TypeString {
		return nil
	}
	if s, _ := a.ReadString(); s != "onMetaData" {
		return nil
	}

	b, _ := a.ReadByte()
	switch b {
	case TypeObject:
		v, _ := a.ReadObject()
		return v
	case TypeEcmaArray:
		v, _ := a.ReadEcmaArray()
		return v
	}

	return nil
}

func (a *AMF0) ReadMap() (map[interface{}]interface{}, error) {
	dict := make(map[interface{}]interface{})

	for a.pos < len(a.buf) {
		k, err := a.ReadItem()
		if err != nil {
			return nil, err
		}
		v, err := a.ReadItem()
		if err != nil {
			return nil, err
		}
		dict[k] = v
	}

	return dict, nil
}

func (a *AMF0) ReadItem() (interface{}, error) {
	dataType, err := a.ReadByte()
	if err != nil {
		return nil, err
	}

	switch dataType {
	case TypeNumber:
		return a.ReadNumber()

	case TypeBoolean:
		v, err := a.ReadByte()
		return v != 0, err

	case TypeString:
		return a.ReadString()

	case TypeObject:
		return a.ReadObject()

	case TypeObjectEnd:
		return nil, nil
	}

	return nil, Err
}

func (a *AMF0) ReadByte() (byte, error) {
	if a.pos >= len(a.buf) {
		return 0, Err
	}

	v := a.buf[a.pos]
	a.pos++
	return v, nil
}

func (a *AMF0) ReadNumber() (float64, error) {
	if a.pos+8 >= len(a.buf) {
		return 0, Err
	}

	v := binary.BigEndian.Uint64(a.buf[a.pos : a.pos+8])
	a.pos += 8
	return math.Float64frombits(v), nil
}

func (a *AMF0) ReadString() (string, error) {
	if a.pos+2 >= len(a.buf) {
		return "", Err
	}

	size := int(binary.BigEndian.Uint16(a.buf[a.pos:]))
	a.pos += 2

	if a.pos+size >= len(a.buf) {
		return "", Err
	}

	s := string(a.buf[a.pos : a.pos+size])
	a.pos += size

	return s, nil
}

func (a *AMF0) ReadObject() (map[string]interface{}, error) {
	obj := make(map[string]interface{})

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

func (a *AMF0) ReadEcmaArray() (map[string]interface{}, error) {
	if a.pos+4 >= len(a.buf) {
		return nil, Err
	}
	a.pos += 4 // skip size

	return a.ReadObject()
}
