package baichuan

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func TestPreviewBurstPreservesPacketBytes(t *testing.T) {
	limits := testLimits(t)
	parser, _ := NewMediaParser(limits)
	client := &Client{previews: make(map[previewKey]*Preview)}
	preview := &Preview{
		client: client, messages: make(chan message, previewQueueMessages),
		queueLimit: int64(limits.MaxMediaBuffer), done: make(chan struct{}), parser: parser,
	}
	// A TCP catch-up burst can contain many small media envelopes.
	const count = 147
	for i := range count {
		frame := make([]byte, 24+(32<<10))
		binary.LittleEndian.PutUint32(frame, mediaPFrameMin)
		copy(frame[4:8], "H264")
		binary.LittleEndian.PutUint32(frame[8:12], 32<<10)
		frame[24] = byte(i)
		preview.deliver(message{payload: frame})
	}
	if queued := preview.queueBytes.Load(); queued == 0 {
		t.Fatal("preview burst was not queued")
	}
	for i := range count {
		packet, err := preview.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if packet.Kind != MediaVideoP || len(packet.Data) != 32<<10 || packet.Data[0] != byte(i) {
			t.Fatalf("packet %d corrupted: kind=%d size=%d first=%d", i, packet.Kind, len(packet.Data), packet.Data[0])
		}
	}
	if queued := preview.queueBytes.Load(); queued != 0 {
		t.Fatalf("preview retained %d queued bytes", queued)
	}
}

func TestPreviewQueueByteBudget(t *testing.T) {
	client := &Client{previews: make(map[previewKey]*Preview)}
	preview := &Preview{
		client: client, messages: make(chan message, previewQueueMessages),
		queueLimit: 32, done: make(chan struct{}),
	}
	preview.deliver(message{payload: make([]byte, 20)})
	preview.deliver(message{payload: make([]byte, 20)})
	if queued := preview.queueBytes.Load(); !errors.Is(preview.Err(), ErrPreviewOverflow) || queued != 20 {
		t.Fatalf("unexpected overflow state: err=%v queued=%d", preview.Err(), queued)
	}
}

func TestPreviewQueueMessageBudget(t *testing.T) {
	client := &Client{previews: make(map[previewKey]*Preview)}
	preview := &Preview{
		client: client, messages: make(chan message, previewQueueMessages),
		queueLimit: 32, done: make(chan struct{}),
	}
	for range previewQueueMessages + 1 {
		preview.deliver(message{})
	}
	if queued := preview.queueBytes.Load(); !errors.Is(preview.Err(), ErrPreviewOverflow) || queued != 0 {
		t.Fatalf("unexpected message overflow state: err=%v queued=%d", preview.Err(), queued)
	}
}
