package hksv

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/stretchr/testify/require"
)

func TestRecordingAttrs(t *testing.T) {
	// Empty or malformed keeps the defaults (nil -> caller uses defaults).
	for _, bad := range []string{"", "1200", "axb", "1200x", "0x1600", "-1x10"} {
		require.Nil(t, recordingAttrs(bad), bad)
	}

	got := recordingAttrs("1200x1600")
	require.Len(t, got, 1)
	require.Equal(t, uint16(1200), got[0].Width)
	require.Equal(t, uint16(1600), got[0].Height)
}

func TestPortraitAdvertised(t *testing.T) {
	// The advertised TLV must actually carry the override, not the default.
	portrait := camera.NewHKSVAccessory("m", "mo", "n", "-", "1", camera.DefaultOperatingState, recordingAttrs("1200x1600")...)
	def := camera.NewHKSVAccessory("m", "mo", "n", "-", "1", camera.DefaultOperatingState)

	var pVal, dVal string
	for _, svc := range portrait.Services {
		for _, ch := range svc.Characters {
			if ch.Type == camera.TypeSupportedVideoRecordingConfiguration {
				pVal, _ = ch.Value.(string)
			}
		}
	}
	for _, svc := range def.Services {
		for _, ch := range svc.Characters {
			if ch.Type == camera.TypeSupportedVideoRecordingConfiguration {
				dVal, _ = ch.Value.(string)
			}
		}
	}

	require.NotEmpty(t, pVal)
	require.NotEqual(t, dVal, pVal, "override must change the advertised configuration")
}
