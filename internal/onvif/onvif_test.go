package onvif

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStreamOnvifWithFragments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ONVIF URL with video only fragment",
			input:    "onvif://user:pass@192.168.1.100#media=video",
			expected: "#media=video",
		},
		{
			name:     "ONVIF URL with multiple fragments",
			input:    "onvif://user:pass@192.168.1.100#video=copy#media=video#backchannel=0",
			expected: "#video=copy#media=video#backchannel=0",
		},
		{
			name:     "ONVIF URL with path and fragments",
			input:    "onvif://user:pass@192.168.1.100:80/onvif/device_service#video=h264#audio=pcma",
			expected: "#video=h264#audio=pcma",
		},
		{
			name:     "ONVIF URL without fragments",
			input:    "onvif://user:pass@192.168.1.100",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test validates that URL parsing preserves fragments
			u, err := url.Parse(tt.input)
			assert.NoError(t, err)

			if tt.expected == "" {
				assert.Empty(t, u.Fragment)
			} else {
				assert.Equal(t, tt.expected[1:], u.Fragment) // Remove leading #
			}
		})
	}
}
