package unifiprotect

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSource(t *testing.T) {
	tests := []struct {
		source string
		want   sourceConfig
	}{
		{
			source: "unifi-protect://02aabbccddee",
			want:   sourceConfig{mac: "02AABBCCDDEE", channel: "video1", audio: true},
		},
		{
			source: "unifi-protect://02AABBCCDDEE?channel=video3&audio=0",
			want:   sourceConfig{mac: "02AABBCCDDEE", channel: "video3", audio: false},
		},
	}

	for _, tt := range tests {
		got, err := parseSource(tt.source)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
}

func TestParseSourceRejectsInvalidValues(t *testing.T) {
	for _, source := range []string{
		"unifi-protect://not-a-mac",
		"unifi-protect://02AABBCCDDEE:7550",
		"unifi-protect://02AABBCCDDEE/video1",
		"unifi-protect://02AABBCCDDEE?channel=video4",
		"unifi-protect://02AABBCCDDEE?audio=yes",
		"unifi-protect://02AABBCCDDEE?unknown=1",
	} {
		_, err := parseSource(source)
		require.Error(t, err, source)
	}
}

func TestNormalizeMAC(t *testing.T) {
	got, err := normalizeMAC("02:aa:bb:cc:dd:ee")
	require.NoError(t, err)
	require.Equal(t, "02AABBCCDDEE", got)
}
