package tuya

import (
	"sort"
	"sync"
)

type FrameBufferQueue struct {
	segments map[int]map[int][]byte // segNum -> fragSeq -> data
	mu       sync.Mutex
}

func NewFrameBufferQueue() *FrameBufferQueue {
	return &FrameBufferQueue{
		segments: make(map[int]map[int][]byte),
	}
}

func (q *FrameBufferQueue) AddFragment(segmentNum, fragmentCount, fragmentSeq int, data []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, ok := q.segments[segmentNum]; !ok {
		q.segments[segmentNum] = make(map[int][]byte)
	}

	q.segments[segmentNum][fragmentSeq] = data
}

func (q *FrameBufferQueue) IsSegmentComplete(segmentNum, fragmentCount int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if frags, ok := q.segments[segmentNum]; ok {
		// Make sure we have the right number of fragments
		if len(frags) != fragmentCount {
			return false
		}

		// Check if we have all sequences from 1 to fragmentCount
		for i := 1; i <= fragmentCount; i++ {
			if _, ok := frags[i]; !ok {
				return false
			}
		}

		return true
	}

	return false
}

func (q *FrameBufferQueue) GetCombinedBuffer(segNum int) []byte {
	q.mu.Lock()
	defer q.mu.Unlock()

	if frags, ok := q.segments[segNum]; ok {
		// Sort fragments by sequence number
		var keys []int
		for k := range frags {
			keys = append(keys, k)
		}
		sort.Ints(keys)

		// Calculate total size for pre-allocation
		totalSize := 0
		for _, k := range keys {
			totalSize += len(frags[k])
		}

		// Pre-allocate buffer for better performance
		combined := make([]byte, 0, totalSize)

		// Combine fragments in sequence order
		for _, k := range keys {
			combined = append(combined, frags[k]...)
		}

		// Remove this segment to free memory
		delete(q.segments, segNum)

		return combined
	}

	return nil
}
