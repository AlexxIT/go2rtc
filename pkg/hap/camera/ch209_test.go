package camera

import (
	"encoding/hex"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/stretchr/testify/require"
)

// Captured from a real HomeKit Home Hub's SelectedCameraRecordingConfiguration
// write (ch209) during a live pairing session. VideoConfig/AudioConfig arrive
// as the chosen config's fields directly - no CodecConfigs array wrapper -
// which is what this test guards against regressing.
func TestSelectedCameraRecordingConfiguration_RealHubPayload(t *testing.T) {
	raw, err := hex.DecodeString(
		"011d0104a00f000002080100000000000000030b01010002060104a00f0000" +
			"022401010002120101010201020304d00700000404a00f0000030b01028007" +
			"0202380403011e0314010100020f010101020100030102040420000000",
	)
	require.NoError(t, err)

	var sel SelectedCameraRecordingConfiguration
	require.NoError(t, tlv8.Unmarshal(raw, &sel))

	require.EqualValues(t, 4000, sel.GeneralConfig.PrebufferLength)
	require.EqualValues(t, 1, sel.GeneralConfig.EventTriggerOptions)
	require.EqualValues(t, 4000, sel.GeneralConfig.MediaContainerConfigurations.MediaContainerParameters.FragmentLength)

	require.EqualValues(t, VideoCodecTypeH264, sel.VideoConfig.CodecType)
	require.Equal(t, []byte{1}, sel.VideoConfig.CodecParams.ProfileID)
	require.Equal(t, []byte{2}, sel.VideoConfig.CodecParams.Level)
	require.Equal(t, []uint32{2000}, sel.VideoConfig.CodecParams.Bitrate)
	require.Equal(t, []uint32{4000}, sel.VideoConfig.CodecParams.IFrameInterval)
	require.Len(t, sel.VideoConfig.CodecAttrs, 1)
	require.EqualValues(t, 1920, sel.VideoConfig.CodecAttrs[0].Width)
	require.EqualValues(t, 1080, sel.VideoConfig.CodecAttrs[0].Height)
	require.EqualValues(t, 30, sel.VideoConfig.CodecAttrs[0].Framerate)

	require.EqualValues(t, AudioRecordingCodecTypeAACLC, sel.AudioConfig.CodecType)
	require.Len(t, sel.AudioConfig.CodecParams, 1)
	require.EqualValues(t, 1, sel.AudioConfig.CodecParams[0].Channels)
}
