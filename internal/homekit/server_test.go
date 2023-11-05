package homekit

import "testing"

func TestCalcDeviceID(t *testing.T) {
	tests := []struct {
		name     string
		deviceID string
		seed     string
		expected string
	}{
		{
			name:     "Empty deviceID and seed",
			deviceID: "",
			seed:     "",
			expected: "46:D2:5E:F2:FE:1A",
		},
		{
			name:     "Non-empty deviceID",
			deviceID: "AA:BB:CC:DD:EE:FF",
			seed:     "seed",
			expected: "AA:BB:CC:DD:EE:FF",
		},
		{
			name:     "Non-empty seed",
			deviceID: "",
			seed:     "seedseedseedseedseedseed",
			expected: "FA:DE:8A:06:BE:0E",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcDeviceID(tt.deviceID, tt.seed)
			if result != tt.expected {
				t.Errorf("Expected %s, but got %s", tt.expected, result)
			}
		})
	}
}
