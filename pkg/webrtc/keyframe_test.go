package webrtc

import "testing"

func TestIsH264KeyframeRTP(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    bool
	}{
		{"single IDR", []byte{0x65, 0x88}, true},
		{"single SPS", []byte{0x67, 0x42}, true},
		{"single P-frame", []byte{0x41, 0x9a}, false},
		{"SEI", []byte{0x06, 0x05}, false},
		{"STAP-A with SPS+PPS", []byte{0x78, 0x00, 0x02, 0x67, 0x42, 0x00, 0x01, 0x68}, true},
		{"STAP-A with SEI only", []byte{0x78, 0x00, 0x02, 0x06, 0x05}, false},
		{"FU-A start IDR", []byte{0x7c, 0x85, 0x88}, true},
		{"FU-A middle IDR", []byte{0x7c, 0x05, 0x88}, false},
		{"FU-A start P-frame", []byte{0x7c, 0x81, 0x9a}, false},
		{"too short", []byte{0x65}, false},
	}
	for _, tt := range tests {
		if got := isH264KeyframeRTP(tt.payload); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
