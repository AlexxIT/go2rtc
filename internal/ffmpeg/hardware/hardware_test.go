package hardware

import (
	"testing"
)

func TestCut(t *testing.T) {
	tests := []struct {
		input    string
		sep      byte
		index    int
		expected string
	}{
		{"apple,banana,cherry", ',', 0, "apple"},
		{"apple,banana,cherry", ',', 1, "banana"},
		{"apple,banana,cherry", ',', 2, "cherry"},
		{"apple,banana,cherry", ',', 3, ""},
		{"", ',', 0, ""},
		{"apple", ',', 0, "apple"},
	}

	for _, tc := range tests {
		result := cut(tc.input, tc.sep, tc.index)
		if result != tc.expected {
			t.Errorf("Expected '%s', but got '%s'", tc.expected, result)
		}
	}
}
