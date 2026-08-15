package homekit

import (
	"encoding/binary"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/stretchr/testify/require"
)

// Real parameter sets from an Aqara G410 doorbell, captured off the wire.
var (
	testSPS = []byte{
		0x27, 0x4d, 0x00, 0x33, 0xe7, 0x40, 0x32, 0x01, 0x2f, 0x4d, 0x40, 0x40,
		0x40, 0x7c, 0x00, 0x00, 0x03, 0x00, 0x04, 0x00, 0x00, 0x03, 0x00, 0xa0,
		0xd4, 0x00, 0x1a, 0x08, 0x00, 0x01, 0x38, 0x60, 0xdf, 0xff, 0xc0, 0xa0,
	}
	testPPS = []byte{0x28, 0xee, 0x3c, 0x80}
)

// stapA builds a STAP-A aggregation packet, the form HomeKit accessories use to
// deliver SPS and PPS ahead of a keyframe.
func stapA(nalus ...[]byte) []byte {
	b := []byte{24} // STAP-A
	for _, nalu := range nalus {
		b = binary.BigEndian.AppendUint16(b, uint16(len(nalu)))
		b = append(b, nalu...)
	}
	return b
}

func TestCollectParameterSets(t *testing.T) {
	tests := []struct {
		name     string
		payloads [][]byte
		sps, pps []byte
	}{
		{
			name:     "stap-a with both",
			payloads: [][]byte{stapA(testSPS, testPPS)},
			sps:      testSPS,
			pps:      testPPS,
		},
		{
			name:     "single nalu each",
			payloads: [][]byte{testSPS, testPPS},
			sps:      testSPS,
			pps:      testPPS,
		},
		{
			name:     "stap-a with unrelated nalu first",
			payloads: [][]byte{stapA([]byte{0x09, 0x10}, testSPS, testPPS)},
			sps:      testSPS,
			pps:      testPPS,
		},
		{
			name: "fragmented payloads ignored",
			// FU-A carrying an IDR - parameter sets are never fragmented, and
			// misreading the fragment header would yield garbage.
			payloads: [][]byte{{28, 0x85, 0x00}},
		},
		{
			name:     "keyframe only, no parameter sets",
			payloads: [][]byte{{0x25, 0xb8, 0x20}},
		},
		{
			name:     "empty payload",
			payloads: [][]byte{{}},
		},
		{
			name: "truncated stap-a does not panic",
			// declares a 500 byte NAL but supplies 2
			payloads: [][]byte{{24, 0x01, 0xf4, 0x67, 0x42}},
		},
		{
			name:     "stap-a with zero length nalu",
			payloads: [][]byte{{24, 0x00, 0x00, 0x67}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sps, pps []byte
			for _, p := range tt.payloads {
				collectParameterSets(p, &sps, &pps)
			}
			require.Equal(t, tt.sps, sps)
			require.Equal(t, tt.pps, pps)
		})
	}
}

func TestCollectParameterSetsKeepsFirst(t *testing.T) {
	var sps, pps []byte
	collectParameterSets(stapA(testSPS, testPPS), &sps, &pps)

	// A later keyframe repeats the parameter sets. They must not be replaced,
	// otherwise the fmtp line would churn while consumers are attaching.
	other := append([]byte(nil), testSPS...)
	other[1] = 0x64
	collectParameterSets(stapA(other, testPPS), &sps, &pps)

	require.Equal(t, testSPS, sps)
}

func TestCollectParameterSetsCopiesPayload(t *testing.T) {
	payload := stapA(testSPS, testPPS)

	var sps, pps []byte
	collectParameterSets(payload, &sps, &pps)

	// RTP payload buffers are reused by the reader, so the parameter sets must
	// be copied rather than aliased.
	for i := range payload {
		payload[i] = 0xff
	}

	require.Equal(t, testSPS, sps)
	require.Equal(t, testPPS, pps)
}

func TestWithParameterSets(t *testing.T) {
	const profile = "profile-level-id=4D0028"

	fmtp := withParameterSets(profile, testSPS, testPPS)
	require.Equal(t, profile+";sprop-parameter-sets=J00AM+dAMgEvTUBAQHwAAAMABAAAAwCg1AAaCAABOGDf/8Cg,KO48gA==", fmtp)

	// already present - must not be appended twice
	require.Equal(t, fmtp, withParameterSets(fmtp, testSPS, testPPS))

	// no existing fmtp line
	require.Equal(t, "sprop-parameter-sets=J00AM+dAMgEvTUBAQHwAAAMABAAAAwCg1AAaCAABOGDf/8Cg,KO48gA==",
		withParameterSets("", testSPS, testPPS))
}

// The fmtp line only helps if the code that consumes it can read it back, so
// assert the round trip through the h264 helper used by RTPDepay.
func TestParameterSetsRoundTrip(t *testing.T) {
	fmtp := withParameterSets("profile-level-id=4D0028", testSPS, testPPS)

	sps, pps := h264.GetParameterSet(fmtp)
	require.Equal(t, testSPS, sps)
	require.Equal(t, testPPS, pps)
}

// Advertising the parameter sets changes what GetProfileLevelID reports, because
// it prefers the SPS over the literal profile-level-id. The G410 negotiates
// level 4.0 over HAP but its SPS says level 5.1, which is outside the set
// GetProfileLevelID accepts, so it falls back to its own default of 4.1. This
// is benign - 4.1 is what go2rtc already advertises for HLS - but it is a
// visible change, so pin it.
func TestProfileLevelIDDerivedFromSPS(t *testing.T) {
	const profile = "profile-level-id=4D0028"

	require.Equal(t, "4D0028", h264.GetProfileLevelID(profile))
	require.Equal(t, "4D0029", h264.GetProfileLevelID(withParameterSets(profile, testSPS, testPPS)))
}

// Without the parameter sets, RTPDepay has nothing to prepend to an IDR - the
// condition that left consumers unable to decode.
func TestParameterSetsAbsentWithoutPatch(t *testing.T) {
	sps, pps := h264.GetParameterSet("profile-level-id=4D0028")
	require.Nil(t, sps)
	require.Nil(t, pps)
}
