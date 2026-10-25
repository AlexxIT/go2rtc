package baichuan

import (
	"bytes"
	"reflect"
	"testing"
)

func TestContiguousPayloads(t *testing.T) {
	store := make(map[uint32][]byte)
	var consumedID uint32
	var hasConsumed bool

	// Initial packet 10
	store[10] = []byte("pkt10")
	chunks := contiguousPayloads(store, &consumedID, &hasConsumed)
	if len(chunks) != 1 || !bytes.Equal(chunks[0], []byte("pkt10")) {
		t.Fatalf("unexpected chunks: %v", chunks)
	}
	if consumedID != 10 || !hasConsumed {
		t.Fatalf("expected consumedID 10, got %d (hasConsumed: %v)", consumedID, hasConsumed)
	}

	// Out of order: 12 and 13 arrive before 11
	store[12] = []byte("pkt12")
	store[13] = []byte("pkt13")
	chunks = contiguousPayloads(store, &consumedID, &hasConsumed)
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks while waiting for pkt11, got %d", len(chunks))
	}

	// Packet 11 arrives: should consume 11, 12, 13
	store[11] = []byte("pkt11")
	chunks = contiguousPayloads(store, &consumedID, &hasConsumed)
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if consumedID != 13 {
		t.Fatalf("expected consumedID 13, got %d", consumedID)
	}
	if len(store) != 0 {
		t.Fatalf("expected store to be empty, got %d items", len(store))
	}
}

func TestAckWindow(t *testing.T) {
	store := make(map[uint32][]byte)

	// Not consumed yet
	_, _, ok := ackWindow(store, 0, false)
	if ok {
		t.Fatalf("expected ackWindow to return false when hasConsumed is false")
	}

	// Nothing missing, no future packets
	start, payload, ok := ackWindow(store, 10, true)
	if !ok || start != 10 || len(payload) != 0 {
		t.Fatalf("expected (10, nil, true), got (%d, %v, %v)", start, payload, ok)
	}

	// Gap: have 12 and 14, missing 11 and 13
	store[12] = []byte("pkt12")
	store[14] = []byte("pkt14")
	start, payload, ok = ackWindow(store, 10, true)
	if !ok || start != 10 {
		t.Fatalf("expected start 10, got %d", start)
	}
	// Missing 11 (0), have 12 (1), missing 13 (0), have 14 (1)
	expectedPayload := []byte{0, 1, 0, 1}
	if !reflect.DeepEqual(payload, expectedPayload) {
		t.Fatalf("expected payload %v, got %v", expectedPayload, payload)
	}
}

func TestAckWindowStaleUnderflowPrevention(t *testing.T) {
	// Scenario reported by user: stale packets <= consumedID remain in store
	// (e.g. retransmitted packets)
	store := make(map[uint32][]byte)
	store[5] = []byte("stale5")

	// consumed is 10. end is 5. If not guarded, end-start underflows uint32 to ~4G.
	start, payload, ok := ackWindow(store, 10, true)
	if !ok {
		t.Fatalf("expected ok true")
	}
	if start != 10 {
		t.Fatalf("expected start 10, got %d", start)
	}
	if len(payload) != 0 {
		t.Fatalf("expected empty payload, got %v", payload)
	}
	// Stale packet <= start should be pruned from store
	if _, exists := store[5]; exists {
		t.Fatalf("expected stale packet 5 to be deleted from store")
	}
}
