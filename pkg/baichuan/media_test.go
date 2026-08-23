package baichuan

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func videoFixture() []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H265")
	binary.LittleEndian.PutUint32(b[8:12], 3)
	binary.LittleEndian.PutUint32(b[12:16], 8)
	binary.LittleEndian.PutUint32(b[16:20], 123456)
	binary.LittleEndian.PutUint32(b[24:28], 1000)
	copy(b[32:35], []byte{1, 2, 3})
	return b
}

func TestMediaParserIFrameExtraHeader(t *testing.T) {
	for _, extra := range []uint32{0, 4, 8, 12} {
		t.Run(fmt.Sprint(extra), func(t *testing.T) {
			const payload = 3
			start := 24 + int(extra)
			video := make([]byte, start+8)
			binary.LittleEndian.PutUint32(video, mediaIFrameMin)
			copy(video[4:8], "H265")
			binary.LittleEndian.PutUint32(video[8:12], payload)
			binary.LittleEndian.PutUint32(video[12:16], extra)
			binary.LittleEndian.PutUint32(video[16:20], 123456)
			if extra >= 4 {
				binary.LittleEndian.PutUint32(video[24:28], 1000)
			}
			copy(video[start:start+payload], []byte{1, 2, 3})

			p, _ := NewMediaParser(Limits{})
			packets, err := p.Append(append(video, infoFixture()...))
			if err != nil || len(packets) != 2 {
				t.Fatalf("unexpected result: packets=%d err=%v", len(packets), err)
			}
			packet := packets[0]
			if !bytes.Equal(packet.Data, []byte{1, 2, 3}) || packet.Timestamp != 123456 {
				t.Fatalf("unexpected packet: %+v", packet)
			}
			if extra >= 4 && packet.WallTime.Unix() != 1000 || extra < 4 && !packet.WallTime.IsZero() {
				t.Fatalf("unexpected wall time: %v", packet.WallTime)
			}
		})
	}
}

func infoFixture() []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b, mediaInfoV2)
	binary.LittleEndian.PutUint32(b[4:8], 32)
	binary.LittleEndian.PutUint32(b[8:12], 3840)
	binary.LittleEndian.PutUint32(b[12:16], 2160)
	b[17] = 20
	return b
}

func TestMediaParserSplitAndOwnership(t *testing.T) {
	p, err := NewMediaParser(Limits{})
	if err != nil {
		t.Fatal(err)
	}
	fixture := videoFixture()
	packets, err := p.Append(fixture[:19])
	if err != nil || len(packets) != 0 {
		t.Fatalf("unexpected partial result: %d, %v", len(packets), err)
	}
	packets, err = p.Append(append(fixture[19:], infoFixture()...))
	if err != nil || len(packets) != 2 {
		t.Fatalf("unexpected complete result: %d, %v", len(packets), err)
	}
	packet := packets[0]
	if packet.Kind != MediaVideoI || packet.Codec != "H265" || packet.Timestamp != 123456 ||
		!packet.WallTime.Equal(packet.WallTime.UTC()) || !bytes.Equal(packet.Data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected packet: %+v", packet)
	}
	fixture[32] = 9
	if packet.Data[0] != 1 {
		t.Fatal("packet data aliases parser input")
	}
}

func TestMediaParserRejectsUnknownPrefix(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	if _, err := p.Append(append([]byte{9, 8, 7, 6}, infoFixture()...)); err == nil || !strings.Contains(err.Error(), "unknown media magic") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMediaParserAdjustsBrokenKeyframeLength(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	video := videoFixture()[:35]
	binary.LittleEndian.PutUint32(video[8:12], 100)
	packets, err := p.Append(append(video, infoFixture()...))
	if err != nil || len(packets) != 2 {
		t.Fatalf("unexpected result: packets=%d err=%v", len(packets), err)
	}
	if !bytes.Equal(packets[0].Data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected keyframe data: %x", packets[0].Data)
	}
}

func TestPreviewBurstPreservesPacketBytes(t *testing.T) {
	limits := testLimits(t)
	parser, _ := NewMediaParser(limits)
	client := &Client{previews: make(map[previewKey]*Preview)}
	preview := &Preview{
		client: client, messages: make(chan message, previewQueueMessages),
		queueLimit: int64(limits.MaxMediaBuffer), done: make(chan struct{}), parser: parser,
	}
	const count = 32
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

func TestMediaParserPFrameExtraHeader(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	b := make([]byte, 36)
	binary.LittleEndian.PutUint32(b, mediaPFrameMin)
	copy(b[4:8], "H265")
	binary.LittleEndian.PutUint32(b[8:12], 3)
	binary.LittleEndian.PutUint32(b[12:16], 4)
	copy(b[28:31], []byte{1, 2, 3})
	packets, err := p.Append(b)
	if err != nil || len(packets) != 1 || !bytes.Equal(packets[0].Data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected P-frame result: %+v err=%v", packets, err)
	}
}

func TestMediaParserRejectsADPCMUnderflow(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, mediaADPCM)
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 2)
	if _, err := p.Append(b); err == nil || !strings.Contains(err.Error(), "ADPCM payload") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMediaParserRejectsInconsistentAudioSize(t *testing.T) {
	p, _ := NewMediaParser(Limits{})
	b := make([]byte, 16)
	binary.LittleEndian.PutUint32(b, mediaAAC)
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 3)
	if _, err := p.Append(b); err == nil || !strings.Contains(err.Error(), "inconsistent audio") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMediaParserUnpaddedFragmentDoesNotConsumeNextHeader(t *testing.T) {
	audio := make([]byte, 13)
	binary.LittleEndian.PutUint32(audio, mediaAAC)
	binary.LittleEndian.PutUint16(audio[4:6], 5)
	binary.LittleEndian.PutUint16(audio[6:8], 5)
	copy(audio[8:], "audio")
	info := infoFixture()
	p, _ := NewMediaParser(Limits{})
	packets, err := p.Append(append(audio, info[:3]...))
	if err != nil || len(packets) != 0 {
		t.Fatalf("unexpected first fragment: packets=%d err=%v", len(packets), err)
	}
	packets, err = p.Append(info[3:])
	if err != nil || len(packets) != 2 || string(packets[0].Data) != "audio" || packets[1].Kind != MediaInfo {
		t.Fatalf("unexpected completed packets: %+v err=%v", packets, err)
	}
}

func TestMediaParserFrameLimit(t *testing.T) {
	p, err := NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 24)
	binary.LittleEndian.PutUint32(b, mediaPFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 100)
	if _, err = p.Append(b); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}

	p, _ = NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	binary.LittleEndian.PutUint32(b[8:12], 0)
	binary.LittleEndian.PutUint32(b[12:16], ^uint32(0))
	if _, err = p.Append(b); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("unexpected I-frame error: %v", err)
	}

	p, _ = NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	b = make([]byte, 72)
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	binary.LittleEndian.PutUint32(b[8:12], 40)
	if _, err = p.Append(append(b, infoFixture()...)); err == nil ||
		!strings.Contains(err.Error(), "video frame 72 exceeds limit 64") {
		t.Fatalf("accepted adjusted I-frame beyond limit: %v", err)
	}
}

func TestMediaParserScansFragmentedKeyframeLinearly(t *testing.T) {
	const resync = 4096
	p, err := NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 8192, MaxResync: resync})
	if err != nil {
		t.Fatal(err)
	}
	header := make([]byte, 24)
	binary.LittleEndian.PutUint32(header, mediaIFrameMin)
	copy(header[4:8], "H264")
	if _, err = p.Append(header); err != nil {
		t.Fatal(err)
	}
	previous := len(header)
	for range resync + 24 {
		_, err = p.Append([]byte{0})
		if p.scan != 0 {
			if p.scan-previous > 1 {
				t.Fatalf("rescanned %d keyframe offsets", p.scan-previous)
			}
			previous = p.scan
		}
	}
	if err == nil || !strings.Contains(err.Error(), "no keyframe boundary") {
		t.Fatalf("unexpected fragmented keyframe result: %v", err)
	}
}

func TestMediaParserAcceptsKeyframeBoundaryAtResyncLimit(t *testing.T) {
	const resync = 64
	p, err := NewMediaParser(Limits{MaxMediaFrame: 128, MaxMediaBuffer: 216, MaxResync: resync})
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 24+resync+24)
	binary.LittleEndian.PutUint32(b, mediaIFrameMin)
	copy(b[4:8], "H264")
	next := 24 + resync
	binary.LittleEndian.PutUint32(b[next:], mediaPFrameMin)
	copy(b[next+4:next+8], "H264")
	packets, err := p.Append(b[:next+8])
	if err != nil || len(packets) != 0 {
		t.Fatalf("rejected partial boundary: packets=%d err=%v", len(packets), err)
	}
	packets, err = p.Append(b[next+8:])
	if err != nil || len(packets) != 2 {
		t.Fatalf("unexpected boundary result: packets=%d err=%v", len(packets), err)
	}
}

func TestMediaParserAppendLimit(t *testing.T) {
	p, err := NewMediaParser(Limits{MaxMediaFrame: 64, MaxMediaBuffer: 160, MaxResync: 64})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Append(make([]byte, 161)); err == nil || !strings.Contains(err.Error(), "media buffer exceeds limit 160") {
		t.Fatalf("unexpected append error: %v", err)
	}
	if p.buf != nil {
		t.Fatal("oversized append changed parser state")
	}
}

func FuzzMediaParser(f *testing.F) {
	f.Add(videoFixture())
	f.Add(infoFixture())
	f.Fuzz(func(t *testing.T, data []byte) {
		p, _ := NewMediaParser(Limits{MaxMediaFrame: 4096, MaxMediaBuffer: 8192, MaxResync: 1024})
		_, _ = p.Append(data)
	})
}
