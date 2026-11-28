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

func TestDecodeSPS2(t *testing.T) {
	s := "QgEBIUAAAAMAkAAAAwAAAwCWoAUCAWlnpbkShc1AQIC4QAAAAwBAAAAFFEn/eEAOpgAV+V8IBBA="
	b, err := base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.NotNil(t, sps)
	require.Equal(t, uint16(640), sps.Width())
	require.Equal(t, uint16(360), sps.Height())
}

func TestDecodeSPSConformanceWindow(t *testing.T) {
	// Hikvision DVR: coded 1920x1088, cropped to 1920x1080
	s := "QgEGIWAAAAMAAAMAAAMAAAMAewAAoAPAgBEHy7ve96clEVcqn1KS5uAgICAQ"
	b, err := base64.StdEncoding.DecodeString(s)
	require.Nil(t, err)

	sps := DecodeSPS(b)
	require.NotNil(t, sps)
	require.Equal(t, uint16(1920), sps.Width())
	require.Equal(t, uint16(1080), sps.Height())
}
