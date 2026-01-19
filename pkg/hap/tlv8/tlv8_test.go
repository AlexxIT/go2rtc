package tlv8

import (
	"encoding/hex"
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

	s := `030b010280070202380403011e ff00 030b010200050202d00203011e`
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

	s := `010280070202380403011e ff00 010200050202d00203011e`
	b1, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	require.NoError(t, err)

	err = Unmarshal(b1, &v)
	require.NoError(t, err)

	require.Len(t, v, 2)

	b2, err := Marshal(v)
	require.NoError(t, err)

	require.Equal(t, b1, b2)
}
