package h265

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeSPS(t *testing.T) {
	s := "QgEBAWAAAAMAAAMAAAMAAAMAmaAAoAgBaH+KrTuiS7/8AAQABbAgApMuADN/mAE="
	b, err := base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.NotNil(t, sps)
	require.Equal(t, uint16(5120), sps.Width())
	require.Equal(t, uint16(1440), sps.Height())
}
