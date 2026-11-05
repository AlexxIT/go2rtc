package wyze

import (
	"encoding/hex"
	"testing"
	"time"
)

func TestSDIndexFrameVectors(t *testing.T) {
	for _, test := range []struct {
		opcode uint16
		hex    string
	}{
		{10450, "484c0500d2280100000000000000000003"},
		{10352, "484c0500702800000000000000000000"},
	} {
		frame, err := BuildSDIndexRequest(test.opcode)
		if err != nil || hex.EncodeToString(frame) != test.hex {
			t.Fatalf("opcode %d: vector mismatch", test.opcode)
		}
	}
}

func TestSDIndexRejectsOtherCommandsWithoutConnecting(t *testing.T) {
	for _, opcode := range []uint16{0, 10010, 10150, 10242, 10350, 10452, 65535} {
		if _, err := BuildSDIndexRequest(opcode); err == nil {
			t.Fatalf("opcode %d was accepted", opcode)
		}
		if _, err := (*Client)(nil).ReadSDIndex(opcode, time.Second); err == nil {
			t.Fatalf("opcode %d was accepted by read method", opcode)
		}
	}
}

func TestSDIndexResponseBounds(t *testing.T) {
	frame, _ := hex.DecodeString("484c0500d32803000000000000000000010203")
	if payload, err := parseSDIndexResponse(frame, 10450); err != nil || hex.EncodeToString(payload) != "010203" {
		t.Fatal("valid response rejected")
	}
	for _, broken := range [][]byte{nil, frame[:15], frame[:18], append([]byte("XX"), frame[2:]...)} {
		if _, err := parseSDIndexResponse(broken, 10450); err == nil {
			t.Fatal("malformed or truncated response accepted")
		}
	}
	if _, err := parseSDIndexResponse(frame, 10352); err == nil {
		t.Fatal("wrong response opcode accepted")
	}
}
