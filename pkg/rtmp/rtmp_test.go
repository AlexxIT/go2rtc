package rtmp

import (
	"net"
	"testing"
)

func TestChunkInterleaving(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	conn := &Conn{
		conn:         c2,
		rd:           c2,
		wr:           c2,
		chunks:       map[uint32]*chunk{},
		rdPacketSize: 128,
	}

	go func() {
		// Client writes chunk size 128
		// Video message: length 200, type 9, csid 4
		// Audio message: length 50, type 8, csid 5

		// Chunk 1 of Video: Format 0 (12 bytes header: 1 basic + 11 msg)
		// Basic: fmt 0, csid 4 => 0x04
		// Msg: time 0 (3 bytes), length 200 (3 bytes), type 9 (1 byte), streamID 1 (4 bytes)
		var b []byte
		b = append(b, 0x04)
		b = append(b, 0x00, 0x00, 0x00) // time
		b = append(b, 0x00, 0x00, 0xC8) // length 200
		b = append(b, 0x09)             // type Video
		b = append(b, 0x01, 0x00, 0x00, 0x00) // streamID 1
		videoPart1 := make([]byte, 128)
		for i := range videoPart1 {
			videoPart1[i] = byte(i)
		}
		b = append(b, videoPart1...)

		// Interleaved Audio chunk: Format 0 (12 bytes header)
		// Basic: fmt 0, csid 5 => 0x05
		// Msg: time 10, length 50, type 8, streamID 1
		b = append(b, 0x05)
		b = append(b, 0x00, 0x00, 0x0A)
		b = append(b, 0x00, 0x00, 0x32)
		b = append(b, 0x08)
		b = append(b, 0x01, 0x00, 0x00, 0x00)
		audioPayload := make([]byte, 50)
		for i := range audioPayload {
			audioPayload[i] = 0xAA
		}
		b = append(b, audioPayload...)

		// Chunk 2 of Video: Format 3 (1 byte basic header)
		// Basic: fmt 3, csid 4 => 0xC4
		b = append(b, 0xC4)
		videoPart2 := make([]byte, 72) // 200 - 128 = 72
		for i := range videoPart2 {
			videoPart2[i] = byte(128 + i)
		}
		b = append(b, videoPart2...)

		_, _ = c1.Write(b)
	}()

	// First message received should be Audio because it completed in 1 chunk!
	tagType1, time1, data1, err := conn.readMessage()
	if err != nil {
		t.Fatalf("readMessage 1 failed: %v", err)
	}
	if tagType1 != TypeAudio {
		t.Fatalf("expected TypeAudio (8), got %d", tagType1)
	}
	if len(data1) != 50 {
		t.Fatalf("expected audio length 50, got %d", len(data1))
	}
	if time1 != 10 {
		t.Fatalf("expected audio time 10, got %d", time1)
	}

	// Second message received should be Video once its second chunk arrives!
	tagType2, time2, data2, err := conn.readMessage()
	if err != nil {
		t.Fatalf("readMessage 2 failed: %v", err)
	}
	if tagType2 != TypeVideo {
		t.Fatalf("expected TypeVideo (9), got %d", tagType2)
	}
	if len(data2) != 200 {
		t.Fatalf("expected video length 200, got %d", len(data2))
	}
	if time2 != 0 {
		t.Fatalf("expected video time 0, got %d", time2)
	}
	for i := 0; i < 200; i++ {
		if data2[i] != byte(i) {
			t.Fatalf("video data mismatch at %d: got %d, expected %d", i, data2[i], byte(i))
		}
	}
}

func TestExtendedCSID(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	conn := &Conn{
		conn:         c2,
		rd:           c2,
		wr:           c2,
		chunks:       map[uint32]*chunk{},
		rdPacketSize: 128,
	}

	go func() {
		// Test CSID 70 (2-byte basic header: 0x00, 0x06 (70 - 64 = 6))
		var b []byte
		b = append(b, 0x00, 0x06)
		b = append(b, 0x00, 0x00, 0x05)       // time 5
		b = append(b, 0x00, 0x00, 0x04)       // length 4
		b = append(b, 0x08)                   // type audio
		b = append(b, 0x01, 0x00, 0x00, 0x00) // streamID 1
		b = append(b, 0x11, 0x22, 0x33, 0x44)

		_, _ = c1.Write(b)
	}()

	tagType, timeMS, data, err := conn.readMessage()
	if err != nil {
		t.Fatalf("readMessage failed: %v", err)
	}
	if tagType != TypeAudio || timeMS != 5 || len(data) != 4 {
		t.Fatalf("mismatch: type=%d time=%d len=%d", tagType, timeMS, len(data))
	}
}

