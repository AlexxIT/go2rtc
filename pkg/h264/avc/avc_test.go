package avc

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeConfig(t *testing.T) {
	s := "01640033ffe1000c67640033ac1514a02800f19001000468ee3cb0"
	b, err := hex.DecodeString(s)
	require.Nil(t, err)

	profile, sps, pps := DecodeConfig(b)
	require.NotNil(t, profile)
	require.NotNil(t, sps)
	require.NotNil(t, pps)
}
