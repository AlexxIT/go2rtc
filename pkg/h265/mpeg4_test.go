package h265

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeConfigDropsMalformedInputSafely(t *testing.T) {
	for _, conf := range [][]byte{
		nil,
		{},
		make([]byte, 22),
		append(make([]byte, 23), 0, 1),
		append(append(make([]byte, 23), 0, 0, 1, 0, 2), 0x40),
	} {
		require.NotPanics(t, func() { DecodeConfig(conf) })
	}
}

func TestEncodeConfigRejectsInvalidNALUs(t *testing.T) {
	for _, nalus := range [][3][]byte{
		{},
		{{0x40}, {0x42, 1, 2, 3, 4}, {0x44}},
		{make([]byte, 0x10000), {0x42, 1, 2, 3, 4, 5}, {0x44}},
		{{0x40}, {0x42, 1, 2, 3, 4, 5}, make([]byte, 0x10000)},
	} {
		require.Nil(t, EncodeConfig(nalus[0], nalus[1], nalus[2]))
	}
}

func TestEncodeDecodeConfig(t *testing.T) {
	vps := []byte{0x40, 1, 2}
	sps := []byte{0x42, 1, 2, 3, 4, 5}
	pps := []byte{0x44, 1}

	profile, gotVPS, gotSPS, gotPPS := DecodeConfig(EncodeConfig(vps, sps, pps))
	require.Equal(t, sps[3:6], profile)
	require.True(t, bytes.Equal(vps, gotVPS))
	require.True(t, bytes.Equal(sps, gotSPS))
	require.True(t, bytes.Equal(pps, gotPPS))
}

func TestEncodeConfigUsesFullNALULengths(t *testing.T) {
	vps := make([]byte, 0xFFFF)
	sps := make([]byte, 0xFFFF)
	pps := make([]byte, 0xFFFF)
	sps[0] = 0x42

	conf := EncodeConfig(vps, sps, pps)
	require.Len(t, conf, 23+5+len(vps)+5+len(sps)+5+len(pps))
	_, gotVPS, gotSPS, gotPPS := DecodeConfig(conf)
	require.Len(t, gotVPS, len(vps))
	require.Len(t, gotSPS, len(sps))
	require.Len(t, gotPPS, len(pps))
}
