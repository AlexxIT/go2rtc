package baichuan

import "sync"

const (
	previewBufferMin  = 32 << 10
	previewBufferMax  = 64 << 10
	previewBufferPage = 8 << 10
)

// The reader takes buffers; Preview.Read returns only bodies copied by the parser.
type previewBuffers struct {
	mu    sync.Mutex
	slots [2][]byte // Oldest first.
}

func previewBufferClass(size int) int {
	if size < previewBufferMin || size > previewBufferMax {
		return 0
	}
	return (size + previewBufferPage - 1) &^ (previewBufferPage - 1)
}

func (b *previewBuffers) get(size int) []byte {
	class := previewBufferClass(size)
	if class == 0 {
		return make([]byte, size)
	}
	b.mu.Lock()
	first, second := b.slots[0], b.slots[1]
	switch class {
	case cap(first):
		b.slots[0], b.slots[1] = second, nil
		b.mu.Unlock()
		return first[:size]
	case cap(second):
		b.slots[1] = nil
		b.mu.Unlock()
		return second[:size]
	}
	b.mu.Unlock()
	return make([]byte, size, class)
}

func (b *previewBuffers) put(body []byte) {
	class := previewBufferClass(len(body))
	if class == 0 || cap(body) != class {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	first, second := b.slots[0], b.slots[1]
	if second != nil && (class == cap(first) || class == cap(second)) {
		return // A return from this class would not improve coverage.
	}
	clear(body[:cap(body)])
	switch {
	case first == nil:
		b.slots[0] = body
	case second == nil:
		b.slots[1] = body
	default:
		b.slots[0], b.slots[1] = second, body
	}
}
