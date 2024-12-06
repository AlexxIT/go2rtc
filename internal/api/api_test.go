package api

import (
	"testing"
	"time"
)

func TestCheckCertExpiration(t *testing.T) {
	// Define a structure for test cases
	tests := []struct {
		name             string
		expirationTime   time.Time     // Input expiration time
		expectedCode     int           // Expected status code returned by the function
		expectedDuration time.Duration // Expected duration until expiration
	}{
		{
			name:             "Expired Certificate",
			expirationTime:   time.Now().Add(-48 * time.Hour), // 2 days ago
			expectedCode:     -1,
			expectedDuration: -48 * time.Hour, // Negative duration indicating past expiration
		},
		{
			name:             "Expiring Today",
			expirationTime:   time.Now().Add(-12 * time.Hour), // 12 hours from now
			expectedCode:     1,
			expectedDuration: -12 * time.Hour, // Positive, less than 24 hours
		},
		{
			name:             "Valid Certificate",
			expirationTime:   time.Now().Add(48 * time.Hour), // 2 days from now
			expectedCode:     0,
			expectedDuration: 48 * time.Hour, // Positive, more than 24 hours
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, duration := checkCertExpiration(tc.expirationTime, "localhost")

			if code != tc.expectedCode {
				t.Errorf("Expected status code %d, got %d", tc.expectedCode, code)
			}

			// Since exact duration comparison can be flaky due to execution time, check if the duration is within a reasonable threshold
			if !durationApproxEquals(duration, tc.expectedDuration, 5*time.Second) {
				t.Errorf("Expected duration %v, got %v", tc.expectedDuration, duration)
			}
		})
	}
}

// durationApproxEquals checks if two durations are approximately equal within a given threshold.
func durationApproxEquals(a, b, threshold time.Duration) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= threshold
}
