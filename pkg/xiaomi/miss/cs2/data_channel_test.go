package cs2

import (
	"encoding/binary"
	"testing"
)

func framedData(payload ...byte) []byte {
	data := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(data, uint32(len(payload)))
	copy(data[4:], payload)
	return data
}

func TestDataChannelMediaQueueRemainsStrict(t *testing.T) {
	channel := newDataChannel(0, 1)
	if err := channel.Push(framedData(1)); err != nil {
		t.Fatal(err)
	}
	if err := channel.Push(framedData(2)); err == nil || err.Error() != "pop buffer is full" {
		t.Fatalf("expected strict full-buffer error, got %v", err)
	}
}

func TestDataChannelControlQueueKeepsNewestMessage(t *testing.T) {
	channel := newDataChannel(0, 1)
	channel.dropOldest = true

	if err := channel.Push(framedData(1)); err != nil {
		t.Fatal(err)
	}
	if err := channel.Push(framedData(2)); err != nil {
		t.Fatal(err)
	}

	data, ok := channel.Pop()
	if !ok {
		t.Fatal("control channel closed unexpectedly")
	}
	if len(data) != 1 || data[0] != 2 {
		t.Fatalf("expected newest control message, got %x", data)
	}
}
