package tlv8

import (
	"encoding/hex"
	"strconv"
	"strings"
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

func TestBytes(t *testing.T) {
	bytes := make([]byte, 255)
	for i := 0; i < len(bytes); i++ {
		bytes[i] = byte(i)
	}

	type Struct struct {
		String string `tlv8:"1"`
	}
	src := Struct{
		String: string(bytes),
	}

	b, err := Marshal(src)
	require.Nil(t, err)

	var dst Struct
	err = Unmarshal(b, &dst)
	require.Nil(t, err)

	require.Equal(t, src, dst)
	require.Equal(t, bytes, []byte(dst.String))
}

func TestVideoCodecParams(t *testing.T) {
	type VideoCodecParams struct {
		ProfileID         []byte `tlv8:"1"`
		Level             []byte `tlv8:"2"`
		PacketizationMode byte   `tlv8:"3"`
		CVOEnabled        []byte `tlv8:"4"`
		CVOID             []byte `tlv8:"5"`
	}

	src, err := hex.DecodeString("0101010201000000020102030100040100")
	require.Nil(t, err)

	var v VideoCodecParams
	err = Unmarshal(src, &v)
	require.Nil(t, err)

	dst, err := Marshal(v)
	require.Nil(t, err)

	require.Equal(t, src, dst)
}

func TestInterface(t *testing.T) {
	type Struct struct {
		Byte byte `tlv8:"1"`
	}

	src := Struct{
		Byte: 1,
	}
	var v1 any = &src

	b, err := Marshal(v1)
	require.Nil(t, err)

	require.Equal(t, []byte{1, 1, 1}, b)

	var dst Struct
	var v2 any = &dst

	err = Unmarshal(b, v2)
	require.Nil(t, err)

	require.Equal(t, src, dst)
}

func TestSlice1(t *testing.T) {
	var v struct {
		VideoAttrs []struct {
			Width     uint16 `tlv8:"1"`
			Height    uint16 `tlv8:"2"`
			Framerate uint8  `tlv8:"3"`
		} `tlv8:"3"`
	}

	s := `030b010280070202380403011e 0000 030b010200050202d00203011e`
	b1, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)

	err = Unmarshal(b1, &v)
	require.NoError(t, err)

	require.Len(t, v.VideoAttrs, 2)

	b2, err := Marshal(v)
	require.NoError(t, err)

	require.Equal(t, b1, b2)
}

func TestSlice2(t *testing.T) {
	var v []struct {
		Width     uint16 `tlv8:"1"`
		Height    uint16 `tlv8:"2"`
		Framerate uint8  `tlv8:"3"`
	}

	s := `010280070202380403011e 0000 010200050202d00203011e`
	b1, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)

	err = Unmarshal(b1, &v)
	require.NoError(t, err)

	require.Len(t, v, 2)

	b2, err := Marshal(v)
	require.NoError(t, err)

	require.Equal(t, b1, b2)
}

func TestBool(t *testing.T) {
	type Struct struct {
		True  bool `tlv8:"1"`
		False bool `tlv8:"2"`
	}

	src := Struct{True: true, False: false}

	b, err := Marshal(src)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1, 2, 1, 0}, b)

	var dst Struct
	err = Unmarshal(b, &dst)
	require.NoError(t, err)

	require.Equal(t, src, dst)
}

func TestBigNestedStruct(t *testing.T) {
	type Inner struct {
		Data string `tlv8:"1"`
	}
	type Outer struct {
		In Inner `tlv8:"2"`
	}

	src := Outer{In: Inner{Data: strings.Repeat("x", 300)}}

	b, err := Marshal(src)
	require.NoError(t, err)

	// nested value is 304 bytes, so it must be split into a 255-byte
	// fragment plus a 49-byte remainder, both tagged 2
	require.Equal(t, byte(2), b[0])
	require.Equal(t, byte(255), b[1])
	require.Equal(t, byte(2), b[257])
	require.Equal(t, byte(49), b[258])

	var dst Outer
	err = Unmarshal(b, &dst)
	require.NoError(t, err)

	require.Equal(t, src, dst)
}

func TestBigArray(t *testing.T) {
	type Struct struct {
		Data [300]byte `tlv8:"1"`
	}

	var src Struct
	for i := range src.Data {
		src.Data[i] = byte(i)
	}

	b, err := Marshal(src)
	require.NoError(t, err)

	var dst Struct
	err = Unmarshal(b, &dst)
	require.NoError(t, err)

	require.Equal(t, src, dst)
}

// Readers must accept any zero-length TLV as a list separator, whatever byte
// the remote implementation picked. Encoding uses 0x00 (see const separator).
func TestSeparatorLeniency(t *testing.T) {
	type Item struct {
		Width  uint16 `tlv8:"1"`
		Height uint16 `tlv8:"2"`
	}

	for _, sep := range []string{"0000", "ff00", "0500"} {
		t.Run(sep, func(t *testing.T) {
			s := `01028007 02023804` + sep + `01020005 02023804`
			b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
			require.NoError(t, err)

			var v []Item
			err = Unmarshal(b, &v)
			require.NoError(t, err)

			require.Equal(t, []Item{
				{Width: 1920, Height: 1080},
				{Width: 1280, Height: 1080},
			}, v)
		})
	}
}

// TestExact255Fragment documents the boundary described on appendPayload: a
// value that fills its last fragment round-trips as the final item, and a
// following same-tag item would be absorbed into it.
func TestExact255Fragment(t *testing.T) {
	type Struct struct {
		Data string `tlv8:"1"`
	}

	for _, n := range []int{254, 255, 256, 510} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			src := Struct{Data: strings.Repeat("y", n)}

			b, err := Marshal(src)
			require.NoError(t, err)

			var dst Struct
			err = Unmarshal(b, &dst)
			require.NoError(t, err)

			require.Equal(t, src, dst)
		})
	}
}
