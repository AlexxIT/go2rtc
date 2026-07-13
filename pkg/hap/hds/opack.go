package hds

import (
	"encoding/binary"
	"errors"
	"math"
)

// opack tags
const (
	opackTrue       = 0x01
	opackFalse      = 0x02
	opackTerminator = 0x03
	opackNull       = 0x04
	opackIntNeg1    = 0x07
	opackSmallInt0  = 0x08 // 0x08-0x2F = integers 0-39
	opackSmallInt39 = 0x2F
	opackInt8       = 0x30
	opackInt16      = 0x31
	opackInt32      = 0x32
	opackInt64      = 0x33
	opackFloat32    = 0x35
	opackFloat64    = 0x36
	opackStr0       = 0x40 // 0x40-0x60 = inline string, length 0-32
	opackStr32      = 0x60
	opackStrLen1    = 0x61
	opackStrLen2    = 0x62
	opackStrLen4    = 0x63
	opackStrLen8    = 0x64
	opackData0      = 0x70 // 0x70-0x90 = inline data, length 0-32
	opackData32     = 0x90
	opackDataLen1   = 0x91
	opackDataLen2   = 0x92
	opackDataLen4   = 0x93
	opackDataLen8   = 0x94
	opackArr0       = 0xD0 // 0xD0-0xDE = counted array, 0-14 elements
	opackArr14      = 0xDE
	opackArrTerm    = 0xDF // terminated array
	opackDict0      = 0xE0 // 0xE0-0xEE = counted dict, 0-14 pairs
	opackDict14     = 0xEE
	opackDictTerm   = 0xEF // terminated dict
)

func OpackMarshal(v any) []byte {
	var buf []byte
	return opackEncode(buf, v)
}

func OpackUnmarshal(data []byte) (any, error) {
	v, _, err := opackDecode(data)
	return v, err
}

func opackEncode(buf []byte, v any) []byte {
	switch v := v.(type) {
	case nil:
		return append(buf, opackNull)
	case bool:
		if v {
			return append(buf, opackTrue)
		}
		return append(buf, opackFalse)
	case int:
		return opackEncodeInt(buf, int64(v))
	case int8:
		return opackEncodeInt(buf, int64(v))
	case int16:
		return opackEncodeInt(buf, int64(v))
	case int32:
		return opackEncodeInt(buf, int64(v))
	case int64:
		return opackEncodeInt(buf, v)
	case uint:
		return opackEncodeInt(buf, int64(v))
	case uint8:
		return opackEncodeInt(buf, int64(v))
	case uint16:
		return opackEncodeInt(buf, int64(v))
	case uint32:
		return opackEncodeInt(buf, int64(v))
	case uint64:
		return opackEncodeInt(buf, int64(v))
	case float32:
		buf = append(buf, opackFloat32)
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, math.Float32bits(v))
		return append(buf, b...)
	case float64:
		buf = append(buf, opackFloat64)
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, math.Float64bits(v))
		return append(buf, b...)
	case string:
		return opackEncodeString(buf, v)
	case []byte:
		return opackEncodeData(buf, v)
	case []any:
		return opackEncodeArray(buf, v)
	case map[string]any:
		return opackEncodeDict(buf, v)
	default:
		return append(buf, opackNull)
	}
}

func opackEncodeInt(buf []byte, v int64) []byte {
	if v == -1 {
		return append(buf, opackIntNeg1)
	}
	if v >= 0 && v <= 39 {
		return append(buf, byte(opackSmallInt0+v))
	}
	if v >= -128 && v <= 127 {
		return append(buf, opackInt8, byte(v))
	}
	if v >= -32768 && v <= 32767 {
		buf = append(buf, opackInt16)
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(v))
		return append(buf, b...)
	}
	if v >= -2147483648 && v <= 2147483647 {
		buf = append(buf, opackInt32)
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(v))
		return append(buf, b...)
	}
	buf = append(buf, opackInt64)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(v))
	return append(buf, b...)
}

func opackEncodeString(buf []byte, s string) []byte {
	n := len(s)
	if n <= 32 {
		buf = append(buf, byte(opackStr0+n))
	} else if n <= 0xFF {
		buf = append(buf, opackStrLen1, byte(n))
	} else if n <= 0xFFFF {
		buf = append(buf, opackStrLen2)
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(n))
		buf = append(buf, b...)
	} else {
		buf = append(buf, opackStrLen4)
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(n))
		buf = append(buf, b...)
	}
	return append(buf, s...)
}

func opackEncodeData(buf []byte, data []byte) []byte {
	n := len(data)
	if n <= 32 {
		buf = append(buf, byte(opackData0+n))
	} else if n <= 0xFF {
		buf = append(buf, opackDataLen1, byte(n))
	} else if n <= 0xFFFF {
		buf = append(buf, opackDataLen2)
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(n))
		buf = append(buf, b...)
	} else {
		buf = append(buf, opackDataLen4)
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, uint32(n))
		buf = append(buf, b...)
	}
	return append(buf, data...)
}

func opackEncodeArray(buf []byte, arr []any) []byte {
	n := len(arr)
	if n <= 14 {
		buf = append(buf, byte(opackArr0+n))
	} else {
		buf = append(buf, opackArrTerm)
	}
	for _, v := range arr {
		buf = opackEncode(buf, v)
	}
	if n > 14 {
		buf = append(buf, opackTerminator)
	}
	return buf
}

func opackEncodeDict(buf []byte, dict map[string]any) []byte {
	n := len(dict)
	if n <= 14 {
		buf = append(buf, byte(opackDict0+n))
	} else {
		buf = append(buf, opackDictTerm)
	}
	for k, v := range dict {
		buf = opackEncodeString(buf, k)
		buf = opackEncode(buf, v)
	}
	if n > 14 {
		buf = append(buf, opackTerminator)
	}
	return buf
}

var errOpackTruncated = errors.New("opack: truncated data")
var errOpackInvalidTag = errors.New("opack: invalid tag")

func opackDecode(data []byte) (any, int, error) {
	if len(data) == 0 {
		return nil, 0, errOpackTruncated
	}

	tag := data[0]
	off := 1

	switch {
	case tag == opackNull:
		return nil, off, nil
	case tag == opackTrue:
		return true, off, nil
	case tag == opackFalse:
		return false, off, nil
	case tag == opackTerminator:
		return nil, off, nil
	case tag == opackIntNeg1:
		return int64(-1), off, nil
	case tag >= opackSmallInt0 && tag <= opackSmallInt39:
		return int64(tag - opackSmallInt0), off, nil
	case tag == opackInt8:
		if len(data) < 2 {
			return nil, 0, errOpackTruncated
		}
		return int64(int8(data[1])), 2, nil
	case tag == opackInt16:
		if len(data) < 3 {
			return nil, 0, errOpackTruncated
		}
		v := int16(binary.LittleEndian.Uint16(data[1:3]))
		return int64(v), 3, nil
	case tag == opackInt32:
		if len(data) < 5 {
			return nil, 0, errOpackTruncated
		}
		v := int32(binary.LittleEndian.Uint32(data[1:5]))
		return int64(v), 5, nil
	case tag == opackInt64:
		if len(data) < 9 {
			return nil, 0, errOpackTruncated
		}
		v := int64(binary.LittleEndian.Uint64(data[1:9]))
		return int64(v), 9, nil
	case tag == opackFloat32:
		if len(data) < 5 {
			return nil, 0, errOpackTruncated
		}
		v := math.Float32frombits(binary.LittleEndian.Uint32(data[1:5]))
		return float64(v), 5, nil
	case tag == opackFloat64:
		if len(data) < 9 {
			return nil, 0, errOpackTruncated
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(data[1:9]))
		return v, 9, nil

	// Inline string (0-32 bytes)
	case tag >= opackStr0 && tag <= opackStr32:
		n := int(tag - opackStr0)
		if len(data) < off+n {
			return nil, 0, errOpackTruncated
		}
		return string(data[off : off+n]), off + n, nil

	// String with length prefix
	case tag >= opackStrLen1 && tag <= opackStrLen4:
		n, sz := opackReadLen(data[off:], tag-opackStrLen1+1)
		if sz == 0 {
			return nil, 0, errOpackTruncated
		}
		off += sz
		if len(data) < off+n {
			return nil, 0, errOpackTruncated
		}
		return string(data[off : off+n]), off + n, nil

	// Inline data (0-32 bytes)
	case tag >= opackData0 && tag <= opackData32:
		n := int(tag - opackData0)
		if len(data) < off+n {
			return nil, 0, errOpackTruncated
		}
		b := make([]byte, n)
		copy(b, data[off:off+n])
		return b, off + n, nil

	// Data with length prefix
	case tag >= opackDataLen1 && tag <= opackDataLen4:
		n, sz := opackReadLen(data[off:], tag-opackDataLen1+1)
		if sz == 0 {
			return nil, 0, errOpackTruncated
		}
		off += sz
		if len(data) < off+n {
			return nil, 0, errOpackTruncated
		}
		b := make([]byte, n)
		copy(b, data[off:off+n])
		return b, off + n, nil

	// Counted array (0-14)
	case tag >= opackArr0 && tag <= opackArr14:
		count := int(tag - opackArr0)
		return opackDecodeArray(data[off:], count, false)

	// Terminated array
	case tag == opackArrTerm:
		return opackDecodeArray(data[off:], 0, true)

	// Counted dict (0-14)
	case tag >= opackDict0 && tag <= opackDict14:
		count := int(tag - opackDict0)
		return opackDecodeDict(data[off:], count, false)

	// Terminated dict
	case tag == opackDictTerm:
		return opackDecodeDict(data[off:], 0, true)
	}

	return nil, 0, errOpackInvalidTag
}

// opackReadLen reads a length from data using the given byte count (1=1byte, 2=2bytes, 3=4bytes, 4=8bytes)
func opackReadLen(data []byte, lenBytes byte) (int, int) {
	switch lenBytes {
	case 1:
		if len(data) < 1 {
			return 0, 0
		}
		return int(data[0]), 1
	case 2:
		if len(data) < 2 {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint16(data[:2])), 2
	case 3: // 4-byte length (tag offset 3 = 4 bytes)
		if len(data) < 4 {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint32(data[:4])), 4
	case 4: // 8-byte length
		if len(data) < 8 {
			return 0, 0
		}
		return int(binary.LittleEndian.Uint64(data[:8])), 8
	}
	return 0, 0
}

func opackDecodeArray(data []byte, count int, terminated bool) ([]any, int, error) {
	var arr []any
	off := 0
	for i := 0; terminated || i < count; i++ {
		if off >= len(data) {
			return nil, 0, errOpackTruncated
		}
		if terminated && data[off] == opackTerminator {
			off++
			break
		}
		v, n, err := opackDecode(data[off:])
		if err != nil {
			return nil, 0, err
		}
		arr = append(arr, v)
		off += n
	}
	return arr, off + 1, nil // +1 for outer tag
}

func opackDecodeDict(data []byte, count int, terminated bool) (map[string]any, int, error) {
	dict := make(map[string]any)
	off := 0
	for i := 0; terminated || i < count; i++ {
		if off >= len(data) {
			return nil, 0, errOpackTruncated
		}
		if terminated && data[off] == opackTerminator {
			off++
			break
		}
		// key
		k, n, err := opackDecode(data[off:])
		if err != nil {
			return nil, 0, err
		}
		off += n

		key, ok := k.(string)
		if !ok {
			return nil, 0, errors.New("opack: dict key is not string")
		}

		// value
		if off >= len(data) {
			return nil, 0, errOpackTruncated
		}
		v, n2, err := opackDecode(data[off:])
		if err != nil {
			return nil, 0, err
		}
		off += n2
		dict[key] = v
	}
	return dict, off + 1, nil // +1 for outer tag
}
