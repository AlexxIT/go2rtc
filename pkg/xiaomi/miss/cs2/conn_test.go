package cs2

import (
	"encoding/binary"
	"testing"
)

func frame(size uint32, payload int) []byte {
	b := make([]byte, 4+payload)
	binary.BigEndian.PutUint32(b, size)
	return b
}

func TestPushFrameSizeBounds(t *testing.T) {
	tests := []struct {
		name    string
		size    uint32
		payload int
		wantErr string
	}{
		{"exactly at the bound", maxFrameSize, maxFrameSize, ""},
		{"one byte over the bound", maxFrameSize + 1, 0, "cs2: invalid frame size 16777217"},
		{"uint32 max stays positive", 0xFFFFFFFF, 0, "cs2: invalid frame size 4294967295"},
		{"negative when cast to int32", 0x80000000, 0, "cs2: invalid frame size 2147483648"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newDataChannel(0, 1)
			err := c.Push(frame(tt.size, tt.payload))

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			case tt.wantErr != "" && err.Error() != tt.wantErr:
				t.Fatalf("expected error %q, got %q", tt.wantErr, err)
			}

			if tt.wantErr != "" && c.waitData != nil {
				t.Fatal("waitData must be dropped after a rejected frame")
			}
		})
	}
}

func TestPushZeroSizeIsPadding(t *testing.T) {
	c := newDataChannel(0, 1)
	payload := []byte("frame")

	if err := c.Push(append(frame(0, 0), frame(uint32(len(payload)), 0)...)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := c.Push(payload); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := c.Pop()
	if !ok || string(data) != "frame" {
		t.Fatalf("expected %q, got %q (ok=%v)", payload, data, ok)
	}
}

func TestPushReassemblesSplitWrites(t *testing.T) {
	c := newDataChannel(0, 2)
	payload := make([]byte, 100)
	for i := range payload {
		payload[i] = byte(i)
	}

	wire := append(frame(uint32(len(payload)), 0), payload...)
	for i := 0; i < len(wire); i += 7 {
		end := min(i+7, len(wire))
		if err := c.Push(wire[i:end]); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	data, ok := c.Pop()
	if !ok || string(data) != string(payload) {
		t.Fatalf("reassembled frame mismatch (ok=%v, len=%d)", ok, len(data))
	}
	if c.waitData != nil {
		t.Fatal("waitData must be released once fully consumed")
	}
}
