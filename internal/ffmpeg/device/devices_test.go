package device

import (
	"testing"
)

func TestShellEscape(t *testing.T) {
	tests := []struct {
		arg      string
		expected string
	}{
		{"test", "'test'"},
		{"hello world", "'hello world'"},
		{"'single quotes'", "''\\''single quotes'\\'''"},
		{"\"double quotes\"", "'\"double quotes\"'"},
		{"single ' quote", "'single '\\'' quote'"},
	}

	for _, test := range tests {
		result := shellEscape(test.arg)
		if result != test.expected {
			t.Errorf("Expected %s, but got %s for input %s", test.expected, result, test.arg)
		}
	}
}
