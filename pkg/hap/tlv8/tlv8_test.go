package tlv8

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshal(t *testing.T) {
	type Struct struct {
		Byte    byte    `tlv8:"1"`
		Uint16  uint16  `tlv8:"2"`
		Uint32  uint32  `tlv8:"3"`
		Float32 float32 `tlv8:"4"`
		String  string  `tlv8:"5"`
		Slice   []byte  `tlv8:"6"`
		Array   [4]byte `tlv8:"7"`
	}

	src := Struct{
		Byte:    1,
		Uint16:  2,
		Uint32:  3,
		Float32: 1.23,
		String:  "123",
		Slice:   []byte{1, 2, 3},
		Array:   [4]byte{1, 2, 3, 4},
	}

	b, err := Marshal(src)
	require.Nil(t, err)

	var dst Struct
	err = Unmarshal(b, &dst)
	require.Nil(t, err)

	require.Equal(t, src, dst)
}
