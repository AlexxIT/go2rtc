package homekit

import (
	"testing"
)

func TestMaxByteSlice(t *testing.T) {
	tests := []struct {
		name    string
		slice   []byte
		wantMax byte
	}{
		{
			name:    "empty slice",
			slice:   []byte{},
			wantMax: 0,
		},
		{
			name:    "single element",
			slice:   []byte{42},
			wantMax: 42,
		},
		{
			name:    "multiple elements",
			slice:   []byte{1, 3, 2},
			wantMax: 3,
		},
		{
			name:    "all elements same",
			slice:   []byte{9, 9, 9},
			wantMax: 9,
		},
		{
			name:    "maximum at start",
			slice:   []byte{99, 10, 88},
			wantMax: 99,
		},
		{
			name:    "maximum at end",
			slice:   []byte{12, 24, 48},
			wantMax: 48,
		},
		// Optionally add another case for multiple occurrences of the maximum value
		{
			name:    "maximum occurs multiple times",
			slice:   []byte{23, 56, 23, 56},
			wantMax: 56,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotMax := maxByteSlice(tt.slice); gotMax != tt.wantMax {
				t.Errorf("maxByteSlice(%v) = %v, want %v", tt.slice, gotMax, tt.wantMax)
			}
		})
	}
}
