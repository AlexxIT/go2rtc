package streams

import (
	"net/url"
	"reflect"
	"testing"
)

// TestParseQuery tests the ParseQuery function with various inputs.
func TestParseQuery(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected url.Values
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:  "single param without value",
			input: "key=",
			expected: url.Values{
				"key": {""},
			},
		},
		{
			name:  "single param with value",
			input: "key=value",
			expected: url.Values{
				"key": {"value"},
			},
		},
		{
			name:  "multiple params",
			input: "key1=value1#key2=value2",
			expected: url.Values{
				"key1": {"value1"},
				"key2": {"value2"},
			},
		},
		{
			name:  "param with no value followed by param with value",
			input: "key1=#key2=value2",
			expected: url.Values{
				"key1": {""},
				"key2": {"value2"},
			},
		},
		{
			name:  "multiple params with one key having multiple values",
			input: "key1=value1#key1=value2",
			expected: url.Values{
				"key1": {"value1", "value2"},
			},
		},
		{
			name:  "svk test",
			input: "http://192.168.88.11/live/files/high/index.m3u8#hardware#video=h264#input=netatmo_input",
			expected: url.Values{
				"hardware": {""},
				"input":    {"netatmo_input"},
				"video":    {"h264"},
				"http://192.168.88.11/live/files/high/index.m3u8": {""},
			},
		},
		{
			name:  "backchannel test",
			input: "ffplay -fflags nobuffer -f alaw -ar 8000 -i -#backchannel=1",
			expected: url.Values{
				"backchannel": {"1"},
				"ffplay -fflags nobuffer -f alaw -ar 8000 -i -": {""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseQuery(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseQuery(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}
