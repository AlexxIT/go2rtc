package onvif

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractFragment(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedURL      string
		expectedFragment string
	}{
		{
			name:             "ONVIF URL with video only fragment",
			input:            "onvif://user:pass@192.168.1.100#media=video",
			expectedURL:      "onvif://user:pass@192.168.1.100",
			expectedFragment: "#media=video",
		},
		{
			name:             "ONVIF URL with multiple fragments",
			input:            "onvif://user:pass@192.168.1.100#video=copy#media=video#backchannel=0",
			expectedURL:      "onvif://user:pass@192.168.1.100",
			expectedFragment: "#video=copy#media=video#backchannel=0",
		},
		{
			name:             "ONVIF URL with path and fragments",
			input:            "onvif://user:pass@192.168.1.100:80/onvif/device_service#video=h264#audio=pcma",
			expectedURL:      "onvif://user:pass@192.168.1.100:80/onvif/device_service",
			expectedFragment: "#video=h264#audio=pcma",
		},
		{
			name:             "ONVIF URL without fragments",
			input:            "onvif://user:pass@192.168.1.100",
			expectedURL:      "onvif://user:pass@192.168.1.100",
			expectedFragment: "",
		},
		{
			name:             "ONVIF URL with query params and fragments",
			input:            "onvif://user:pass@192.168.1.100?subtype=Profile001#video=copy#media=video#backchannel=0",
			expectedURL:      "onvif://user:pass@192.168.1.100?subtype=Profile001",
			expectedFragment: "#video=copy#media=video#backchannel=0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the extractFragment function
			gotURL, gotFragment := extractFragment(tt.input)

			assert.Equal(t, tt.expectedURL, gotURL)
			assert.Equal(t, tt.expectedFragment, gotFragment)

			// Verify that the extracted URL is valid
			_, err := url.Parse(gotURL)
			assert.NoError(t, err)
		})
	}
}
