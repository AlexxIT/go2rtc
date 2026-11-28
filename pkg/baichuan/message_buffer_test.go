package baichuan

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func (b *previewBuffers) cached() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.slots[1] != nil {
		return 2
	}
	if b.slots[0] != nil {
		return 1
	}
	return 0
}

func TestPreviewBuffersClassesAndZero(t *testing.T) {
	var buffers previewBuffers
	for _, size := range []int{32 << 10, (32 << 10) + 1, 40 << 10, (40 << 10) + 1, 64 << 10} {
		body := buffers.get(size)
		if cap(body) != (size+8191)&^8191 {
			t.Fatalf("size %d got capacity %d", size, cap(body))
		}
		for i := range body {
			body[i] = 0x55
		}
		buffers.put(body)
		reused := buffers.get(size)
		if &reused[0] != &body[0] || !bytes.Equal(reused[:cap(reused)], make([]byte, cap(reused))) {
			t.Fatal("body was not reused after zeroing its used bytes")
		}
	}
	long := buffers.get(40 << 10)
	for i := range long {
		long[i] = 0x55
	}
	buffers.put(long)
	short := buffers.get(33 << 10)
	full := short[:cap(short)]
	for i := range full {
		full[i] = 0x77
	}
	buffers.put(short)
	if restored := buffers.get(40 << 10); &restored[0] != &long[0] || !bytes.Equal(restored, make([]byte, len(restored))) {
		t.Fatal("same-class long/short/long reuse retained a stale tail")
	}
	large := buffers.get(64 << 10)
	buffers.put(large)
	if smaller := buffers.get(33 << 10); cap(smaller) != 40<<10 || &smaller[0] == &large[0] {
		t.Fatal("cache reused a different capacity class")
	}
	if restored := buffers.get(len(large)); &restored[0] != &large[0] {
		t.Fatal("cache discarded an unmatched buffer")
	}
	for _, size := range []int{(32 << 10) - 1, (64 << 10) + 1} {
		body := buffers.get(size)
		buffers.put(body)
		if buffers.cached() != 0 || cap(body) != size {
			t.Fatalf("ineligible size %d entered the cache", size)
		}
	}
}

func TestPreviewBuffersAlternatingClasses(t *testing.T) {
	const smallSize, largeSize = 33 << 10, 49 << 10
	var buffers previewBuffers
	older := buffers.get(smallSize)
	small := buffers.get(smallSize)
	large := buffers.get(largeSize)
	done := make(chan struct{})
	go func() {
		buffers.put(older)
		buffers.put(small)
		buffers.put(large)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("full buffer cache blocked release")
	}
	if buffers.cached() != 2 {
		t.Fatal("cache did not retain exactly two buffers")
	}

	spare := make([]byte, smallSize, cap(small))
	buffers.put(spare)
	for range 4 {
		for _, original := range [][]byte{large, small} {
			reused := buffers.get(len(original))
			if &reused[0] != &original[0] {
				t.Fatalf("cache missed a warmed class of capacity %d", cap(original))
			}
			buffers.put(reused)
		}
	}

	third := buffers.get(64 << 10)
	buffers.put(third)
	if restored := buffers.get(len(small)); &restored[0] != &small[0] {
		t.Fatal("a third class evicted the more recently returned buffer")
	}
	if restored := buffers.get(len(third)); &restored[0] != &third[0] {
		t.Fatal("a third class could not replace the oldest buffer")
	}
}

func bufferPreview(t *testing.T) *Preview {
	t.Helper()
	limits := testLimits(t)
	parser, err := NewMediaParser(limits)
	if err != nil {
		t.Fatal(err)
	}
	return &Preview{client: &Client{},
		messages: make(chan message, previewQueueMessages), queueLimit: int64(limits.MaxMediaBuffer),
		done: make(chan struct{}), parser: parser}
}

func bufferVideo() []byte {
	video := bytes.Repeat([]byte{0x55}, 24+(48<<10))
	binary.LittleEndian.PutUint32(video, mediaPFrameMin)
	copy(video[4:], "H264")
	binary.LittleEndian.PutUint32(video[8:], 48<<10)
	binary.LittleEndian.PutUint32(video[12:], 0)
	return video
}

func TestPreviewBuffersCopiedAndAdoptedOwnership(t *testing.T) {
	for _, residue := range []bool{false, true} {
		p := bufferPreview(t)
		video := bufferVideo()
		want := bytes.Clone(video[24:])
		incoming := video
		if residue {
			if _, err := p.parser.appendOwned(video[:1024]); err != nil {
				t.Fatal(err)
			}
			incoming = video[1024:]
		}
		body := p.client.readBuffers.get(16 + len(incoming))
		copy(body[:16], bytes.Repeat([]byte{0x77}, 16))
		copy(body[16:], incoming)
		p.deliver(message{extension: body[:16], payload: body[16:], recycleBody: body})
		packet, err := p.Read(context.Background())
		if err != nil || !bytes.Equal(packet.Data, want) {
			t.Fatal("preview packet changed before reuse", err)
		}
		wantCached := 0
		if residue {
			wantCached = 1
			if !bytes.Equal(body, make([]byte, len(body))) {
				t.Fatal("copied body retained extension or payload bytes")
			}
		}
		if p.client.readBuffers.cached() != wantCached {
			t.Fatal("adopted body was released or copied body was retained")
		}
		reused := p.client.readBuffers.get(len(body))
		for i := range reused {
			reused[i] = 0x99
		}
		if !bytes.Equal(packet.Data, want) {
			t.Fatal("reusing a body corrupted a previously returned packet")
		}
	}
}

func TestPreviewBuffersKeepPendingResponseBody(t *testing.T) {
	clientConn, camera := net.Pipe()
	cfg, err := NewConfig("camera.local", "user", "password").normalized()
	if err != nil {
		t.Fatal(err)
	}
	client := newClient(context.Background(), cfg, clientConn)
	t.Cleanup(func() { _ = client.Close(); _ = camera.Close() })
	_ = camera.SetDeadline(time.Now().Add(time.Second))
	key, pending, err := client.reserve(request{command: commandPreview})
	if err != nil {
		t.Fatal(err)
	}
	defer client.release(key, pending)
	p := bufferPreview(t)
	p.client = client
	if err = client.addPreview(p); err != nil {
		t.Fatal(err)
	}
	video := bufferVideo()
	want := bytes.Clone(video[24:])
	wantResponse := bytes.Clone(video[1024:])
	if _, err = p.parser.appendOwned(video[:1024]); err != nil {
		t.Fatal(err)
	}
	if err = writeTestFrame(camera, header{Command: commandPreview, Sequence: key.sequence, ResponseCode: 200},
		[]byte(binaryExtensionXML), video[1024:], cipherState{}, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	packet, err := p.Read(ctx)
	if err != nil || !bytes.Equal(packet.Data, want) {
		t.Fatal("pending startup media changed", err)
	}
	select {
	case response := <-pending.response:
		if response.recycleBody != nil || !bytes.Equal(response.payload, wantResponse) || client.readBuffers.cached() != 0 {
			t.Fatal("parser recycling corrupted the pending request response")
		}
	case <-ctx.Done():
		t.Fatal("binary startup response did not complete")
	}
}
