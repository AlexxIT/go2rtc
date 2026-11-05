package cs2

import (
	"bytes"
	"io"
	"net"
	"testing"
)

func TestWritePacketIncludesPayload(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	header := bytes.Repeat([]byte{0x11}, hdrSize)
	payload := []byte{0xBA, 0x03, 0x10, 0x20, 0x30, 0x40}
	conn := &Conn{Conn: client}

	done := make(chan error, 1)
	go func() {
		done <- conn.WritePacket(header, payload)
	}()

	packet := make([]byte, 12+hdrSize+len(payload))
	if _, err := io.ReadFull(server, packet); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if got := packet[12 : 12+hdrSize]; !bytes.Equal(got, header) {
		t.Fatalf("media header mismatch: %x", got)
	}
	if got := packet[12+hdrSize:]; !bytes.Equal(got, payload) {
		t.Fatalf("audio payload mismatch: got %x, want %x", got, payload)
	}
}
