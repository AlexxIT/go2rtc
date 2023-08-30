package tlv8

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
)

type errReader struct {
	err error
}

func (e *errReader) Read([]byte) (int, error) {
	return 0, e.err
}

func MarshalBase64(v any) (string, error) {
	b, err := Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func MarshalReader(v any) io.Reader {
	b, err := Marshal(v)
	if err != nil {
		return &errReader{err: err}
	}
	return bytes.NewReader(b)
}

func Marshal(v any) ([]byte, error) {
	value := reflect.ValueOf(v)
	kind := value.Type().Kind()

	if kind == reflect.Pointer {
		value = value.Elem()
		kind = value.Type().Kind()
	}

	switch kind {
	case reflect.Struct:
		return appendStruct(nil, value)
	}

	return nil, errors.New("tlv8: not implemented: " + kind.String())
}

func appendStruct(b []byte, value reflect.Value) ([]byte, error) {
	valueType := value.Type()

	for i := 0; i < value.NumField(); i++ {
		refField := value.Field(i)
		s, ok := valueType.Field(i).Tag.Lookup("tlv8")
		if !ok {
			continue
		}

		tag, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}

		b, err = appendValue(b, byte(tag), refField)
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

func appendValue(b []byte, tag byte, value reflect.Value) ([]byte, error) {
	var err error

	switch value.Kind() {
	case reflect.Uint8:
		v := value.Uint()
		return append(b, tag, 1, byte(v)), nil

	case reflect.Uint16:
		v := value.Uint()
		return append(b, tag, 2, byte(v), byte(v>>8)), nil

	case reflect.Uint32:
		v := value.Uint()
		return append(b, tag, 4, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)), nil

	case reflect.Float32:
		v := math.Float32bits(float32(value.Float()))
		return append(b, tag, 4, byte(v), byte(v>>8), byte(v>>16), byte(v>>24)), nil

	case reflect.String:
		v := value.String()
		l := len(v) // support "big" string
		for ; l > 255; l -= 255 {
			b = append(b, tag, 255)
			b = append(b, v[:255]...)
			v = v[255:]
		}
		b = append(b, tag, byte(l))
		return append(b, v...), nil

	case reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			n := value.Len()
			b = append(b, tag, byte(n))
			for i := 0; i < n; i++ {
				b = append(b, byte(value.Index(i).Uint()))
			}
			return b, nil
		}

	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if i > 0 {
				b = append(b, 0, 0)
			}
			if b, err = appendValue(b, tag, value.Index(i)); err != nil {
				return nil, err
			}
		}
		return b, nil

	case reflect.Struct:
		b = append(b, tag, 0)
		i := len(b)
		if b, err = appendStruct(b, value); err != nil {
			return nil, err
		}
		b[i-1] = byte(len(b) - i) // set struct size
		return b, nil
	}

	return nil, errors.New("tlv8: not implemented: " + value.Kind().String())
}

func UnmarshalBase64(s string, v any) error {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return Unmarshal(data, v)
}

func UnmarshalReader(r io.Reader, v any) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return Unmarshal(data, v)
}

func Unmarshal(data []byte, v any) error {
	if len(data) == 0 {
		return errors.New("tlv8: unmarshal zero data")
	}

	value := reflect.ValueOf(v)
	kind := value.Kind()

	if kind != reflect.Pointer {
		return errors.New("tlv8: value should be pointer: " + kind.String())
	}

	value = value.Elem()
	kind = value.Kind()

	if kind == reflect.Interface {
		value = value.Elem()
		kind = value.Kind()
	}

	if kind != reflect.Struct {
		return errors.New("tlv8: not implemented: " + kind.String())
	}

	return unmarshalStruct(data, value)
}

func unmarshalStruct(b []byte, value reflect.Value) error {
	var waitSlice bool

	for len(b) >= 2 {
		t := b[0]
		l := int(b[1])

		// array item divider
		if t == 0 && l == 0 {
			b = b[2:]
			waitSlice = true
			continue
		}

		var v []byte

		for {
			if len(b) < 2+l {
				return errors.New("tlv8: wrong size: " + value.Type().Name())
			}

			v = append(v, b[2:2+l]...)
			b = b[2+l:]

			// if size == 255 and same tag - continue read big payload
			if l < 255 || len(b) < 2 || b[0] != t {
				break
			}

			l = int(b[1])
		}

		tag := strconv.Itoa(int(t))

		valueField, ok := getStructField(value, tag)
		if !ok {
			return fmt.Errorf("tlv8: can't find T=%d,L=%d,V=%x for: %s", t, l, v, value.Type().Name())
		}

		if waitSlice {
			if valueField.Kind() != reflect.Slice {
				return fmt.Errorf("tlv8: should be slice T=%d,L=%d,V=%x for: %s", t, l, v, value.Type().Name())
			}
			waitSlice = false
		}

		if err := unmarshalValue(v, valueField); err != nil {
			return err
		}
	}

	return nil
}

func unmarshalValue(v []byte, value reflect.Value) error {
	switch value.Kind() {
	case reflect.Uint8:
		if len(v) != 1 {
			return errors.New("tlv8: wrong size: " + value.Type().Name())
		}
		value.SetUint(uint64(v[0]))

	case reflect.Uint16:
		if len(v) != 2 {
			return errors.New("tlv8: wrong size: " + value.Type().Name())
		}
		value.SetUint(uint64(v[0]) | uint64(v[1])<<8)

	case reflect.Uint32:
		if len(v) != 4 {
			return errors.New("tlv8: wrong size: " + value.Type().Name())
		}
		value.SetUint(uint64(v[0]) | uint64(v[1])<<8 | uint64(v[2])<<16 | uint64(v[3])<<24)

	case reflect.Float32:
		f := math.Float32frombits(binary.LittleEndian.Uint32(v))
		value.SetFloat(float64(f))

	case reflect.String:
		value.SetString(string(v))

	case reflect.Array:
		if kind := value.Type().Elem().Kind(); kind != reflect.Uint8 {
			return errors.New("tlv8: unsupported array: " + kind.String())
		}

		for i, b := range v {
			value.Index(i).SetUint(uint64(b))
		}
		return nil

	case reflect.Slice:
		i := growSlice(value)
		return unmarshalValue(v, value.Index(i))

	case reflect.Struct:
		return unmarshalStruct(v, value)

	default:
		return errors.New("tlv8: not implemented: " + value.Kind().String())
	}

	return nil
}

func getStructField(value reflect.Value, tag string) (reflect.Value, bool) {
	valueType := value.Type()

	for i := 0; i < value.NumField(); i++ {
		valueField := value.Field(i)

		if s, ok := valueType.Field(i).Tag.Lookup("tlv8"); ok && s == tag {
			return valueField, true
		}
	}

	return reflect.Value{}, false
}

func growSlice(value reflect.Value) int {
	size := value.Len()

	if size >= value.Cap() {
		newcap := value.Cap() + value.Cap()/2
		if newcap < 4 {
			newcap = 4
		}
		newValue := reflect.MakeSlice(value.Type(), value.Len(), newcap)
		reflect.Copy(newValue, value)
		value.Set(newValue)
	}

	if size >= value.Len() {
		value.SetLen(size + 1)
	}

	return size
}
