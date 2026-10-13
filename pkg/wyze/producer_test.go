package wyze

import (
	"encoding/base64"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/stretchr/testify/require"
)

func TestIsSD(t *testing.T) {
	// Wyze Cam v3 (WYZE_CAKP2JFUS) asked for HD: the first SPS is still the 640x360 one
	sd, _ := base64.StdEncoding.DecodeString("J00AMudAUBf8s1AQEB8AAAMAAQAAAwAoMTADqYAFfl//wKA=")
	hd, _ := base64.StdEncoding.DecodeString("J00AKedAPAET8s1AQEB8AAADAAQAAAMAoMkABpeAAnjX//Ao")

	s := h264.DecodeSPS(sd)
	require.Equal(t, uint16(640), s.Width())
	require.Equal(t, uint16(360), s.Height())
	require.True(t, isSD(s))

	s = h264.DecodeSPS(hd)
	require.Equal(t, uint16(1920), s.Width())
	require.Equal(t, uint16(1080), s.Height())
	require.False(t, isSD(s))

	require.False(t, isSD(nil))
}

func TestFirstNALU(t *testing.T) {
	require.Equal(t, []byte{0x67, 1, 2}, firstNALU([]byte{0, 0, 0, 3, 0x67, 1, 2, 0, 0, 0, 1, 0x68}))
	require.Nil(t, firstNALU([]byte{0, 0, 0, 9, 0x67}))
	require.Nil(t, firstNALU([]byte{0, 0}))
}
