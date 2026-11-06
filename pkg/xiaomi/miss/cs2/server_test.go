package cs2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeInitString(t *testing.T) {
	// from miss_get_vendor for midr.cateye.ph300
	s := "EEGGFCBNKAJNGBJOEMGLFJEBHENOHNNEGKFMBFCHABJBLHKDDAAECLPDGILJJHLLAJMOKCDLOANABACHIP"
	servers, err := DecodeInitString(s)
	require.NoError(t, err)
	require.Equal(t, []string{"42.157.163.61", "110.43.68.105", "110.43.39.45"}, servers)
}
