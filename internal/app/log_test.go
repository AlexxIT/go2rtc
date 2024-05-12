package app

import (
	"reflect"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestCircularBuffer(t *testing.T) {
	buf := newBuffer(2) // Small buffer for testing

	// Test writing and wrapping
	msg1 := []byte("hello")
	msg2 := []byte("world")
	_, err := buf.Write(msg1)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	_, err = buf.Write(msg2)
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}

	// Test buffer content
	expected := "helloworld"
	if string(buf.Bytes()) != expected {
		t.Errorf("Expected %s, got %s", expected, string(buf.Bytes()))
	}

	// Test reset
	buf.Reset()
	if len(buf.Bytes()) != 0 {
		t.Errorf("Expected empty buffer after reset, got %d bytes", len(buf.Bytes()))
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		format string
		level  string
	}{
		{"json", "info"},
		{"text", "debug"},
	}

	for _, tc := range tests {
		logger := NewLogger(tc.format, tc.level)

		// Check if logger has the correct level
		lvl := logger.GetLevel()
		expectedLvl, _ := zerolog.ParseLevel(tc.level)
		if lvl != expectedLvl {
			t.Errorf("Expected level %s, got %s", tc.level, lvl.String())
		}

		// Additional checks can be added here for format verification
	}
}

func TestGetLogger(t *testing.T) {
	modules = map[string]string{
		"module1": "debug",
		"module2": "warn",
	}

	logger1 := GetLogger("module1")
	if logger1.GetLevel() != zerolog.DebugLevel {
		t.Errorf("Expected debug level for module1, got %s", logger1.GetLevel().String())
	}

	logger2 := GetLogger("module2")
	if logger2.GetLevel() != zerolog.WarnLevel {
		t.Errorf("Expected warn level for module2, got %s", logger2.GetLevel().String())
	}

	// Test non-existent module (should default to global logger level)
	logger3 := GetLogger("nonexistent")
	if logger3.GetLevel() != log.Logger.GetLevel() {
		t.Errorf("Expected default logger level for nonexistent module, got %s", logger3.GetLevel().String())
	}
}

func TestCircularBuffer_Bytes(t *testing.T) {
	tests := []struct {
		name     string
		buffer   *circularBuffer
		expected []byte
	}{
		{
			name: "empty buffer",
			buffer: &circularBuffer{
				chunks: make([]([]byte), 5),
				r:      0,
				w:      0,
			},
			expected: []byte{},
		},
		{
			name: "partially filled buffer",
			buffer: &circularBuffer{
				chunks: [][]byte{{'a', 'b'}, {'c', 'd'}, {}, {}, {}},
				r:      0,
				w:      2,
			},
			expected: []byte{'a', 'b', 'c', 'd'},
		},
		{
			name: "wrapped around buffer",
			buffer: &circularBuffer{
				chunks: [][]byte{{'e', 'f'}, {'g', 'h'}, {'a', 'b'}, {'c', 'd'}, {}},
				r:      2,
				w:      1,
			},
			expected: []byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.buffer.Bytes(); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("circularBuffer.Bytes() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// BenchmarkCircularBuffer_Bytes benchmarks the Bytes method of circularBuffer.
func BenchmarkCircularBuffer_Bytes(b *testing.B) {
	buffer := &circularBuffer{
		chunks: make([]([]byte), 1024), // Assuming a buffer capacity of 1024 chunks
		r:      0,
		w:      512, // Assuming the buffer is half full for this benchmark
	}

	// Pre-fill the buffer with data to simulate a real-world scenario.
	for i := range buffer.chunks[:buffer.w] {
		buffer.chunks[i] = []byte("some repeated sample data")
	}

	// The actual benchmark loop
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buffer.Bytes()
	}
}
