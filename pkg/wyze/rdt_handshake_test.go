package wyze

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestRDTHandshakeRequiresAuthentication(t *testing.T) {
	for _, client := range []*Client{nil, {}, {closed: true}} {
		if err := client.ProbeRDTHandshake(); err == nil {
			t.Fatal("handshake accepted an unauthenticated/closed client")
		}
	}
}

func TestRDTMalformedACKProducesOnlyNumericDiagnostics(t *testing.T) {
	c := &Client{}
	frame := c.buildK10010(MediaTypeRDT, true)
	binary.LittleEndian.PutUint16(frame[4:], KCmdControlChannelResp)
	frame[2], frame[17] = 4, 2
	raw := append(make([]byte, 32), frame...)
	if !c.matchHL(KCmdControlChannelResp)(raw) {
		t.Fatal("negative/version-mismatched ACK was discarded before diagnostics")
	}
	frame = append(frame, []byte("PRIVATE_RESPONSE_CONTENT")...)
	message := rdtControlDiagnostic(frame).Error()
	for _, part := range []string{"version=4", "payload_len=2", "media=4", "result=2"} {
		if !strings.Contains(message, part) {
			t.Fatalf("missing numeric diagnostic %s", part)
		}
	}
	if strings.Contains(message, "PRIVATE_RESPONSE_CONTENT") {
		t.Fatal("raw ACK content leaked into diagnostic")
	}
}

func TestRDTServerControlFrames(t *testing.T) {
	c := &Client{}
	for _, test := range []struct {
		enabled bool
		payload []byte
	}{{true, []byte{4, 1}}, {false, []byte{4, 2}}} {
		frame := c.buildK10010(MediaTypeRDT, test.enabled)
		if len(frame) != 18 || binary.LittleEndian.Uint16(frame[4:]) != 10010 ||
			binary.LittleEndian.Uint16(frame[6:]) != 2 || !bytes.Equal(frame[16:], test.payload) {
			t.Fatal("RDT prep/cleanup changed its app-verified opcode or payload")
		}
	}
	response := c.buildK10010(MediaTypeRDT, true)
	binary.LittleEndian.PutUint16(response[4:], KCmdControlChannelResp)
	if !isRDTControlResponse(response) {
		t.Fatal("valid media-4 acknowledgement rejected")
	}
	for _, index := range []int{0, 2, 4, 6, 16} {
		invalid := append([]byte(nil), response...)
		invalid[index] ^= 1
		if isRDTControlResponse(invalid) {
			t.Fatalf("invalid control response accepted after changing byte %d", index)
		}
	}
	if isRDTControlResponse(response[:17]) {
		t.Fatal("truncated acknowledgement accepted")
	}
}

func TestRDTControlAcceptsObservedDeviceProtocol37(t *testing.T) {
	c := &Client{}
	response := c.buildK10010(MediaTypeRDT, true)
	binary.LittleEndian.PutUint16(response[4:], KCmdControlChannelResp)
	binary.LittleEndian.PutUint16(response[2:], 37)
	if !isRDTControlResponse(response) {
		t.Fatal("observed firmware 4.36.16.6114 media-4 success acknowledgement rejected")
	}
}
