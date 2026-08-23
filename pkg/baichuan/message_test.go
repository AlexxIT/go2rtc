package baichuan

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

func testLimits(t *testing.T) Limits {
	t.Helper()
	limits, err := (Limits{}).normalized()
	if err != nil {
		t.Fatal(err)
	}
	return limits
}

func TestEncodeNonceRequest(t *testing.T) {
	packet, err := encodeRequest(request{
		command: commandLogin, sequence: 7, class: classLegacy, forceBC: true,
	}, testLimits(t), cipherState{})
	if err != nil {
		t.Fatal(err)
	}
	const expected = "f0debc0a01000000000000000000070012dc1465"
	if actual := hex.EncodeToString(packet); actual != expected {
		t.Fatalf("unexpected packet:\n%s\n%s", actual, expected)
	}
}

func TestReadFrameBounds(t *testing.T) {
	limits := testLimits(t)
	header := make([]byte, 24)
	binary.LittleEndian.PutUint32(header, wireMagic)
	binary.LittleEndian.PutUint32(header[8:], limits.MaxBody+1)
	binary.LittleEndian.PutUint16(header[18:], classOffset)
	if _, err := readFrame(bytes.NewReader(header), limits); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("unexpected error: %v", err)
	}

	binary.LittleEndian.PutUint32(header[8:], 1)
	binary.LittleEndian.PutUint32(header[20:], 2)
	if _, err := readFrame(bytes.NewReader(header), limits); err == nil || !strings.Contains(err.Error(), "payload offset") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadFrameRejectsControlPayloadBeforeRead(t *testing.T) {
	limits := testLimits(t)
	for command, limit := range map[uint32]uint32{
		commandLogin: maxNonceBody, commandTalkAbility: maxTalkAbilityBody,
		commandDeviceInfo: maxDeviceInfoBody, commandAbility: maxCapabilityBody,
	} {
		header := make([]byte, 20)
		binary.LittleEndian.PutUint32(header, wireMagic)
		binary.LittleEndian.PutUint32(header[4:], command)
		binary.LittleEndian.PutUint32(header[8:], limit+1)
		binary.LittleEndian.PutUint16(header[18:], classLegacy)
		if _, err := readFrame(bytes.NewReader(header), limits); err == nil ||
			!strings.Contains(err.Error(), "payload") {
			t.Fatalf("command %d read oversized payload: %v", command, err)
		}
	}
}

func TestDecodeFrame(t *testing.T) {
	state := cipherState{mode: encryptionBC}
	ext := []byte("<body><binaryData>1</binaryData></body>")
	body := append(testCrypt(state, 1, ext, true), []byte{1, 2, 3}...)
	msg, err := decodeFrame(frame{
		header: header{Channel: 1, BodyLen: uint32(len(body)), PayloadOffset: uint32(len(ext))},
		body:   body,
	}, state, false)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.binary || !bytes.Equal(msg.payload, []byte{1, 2, 3}) {
		t.Fatalf("unexpected message: %+v", msg)
	}
}

func TestDecodeFrameRejectsInvalidOffset(t *testing.T) {
	_, err := decodeFrame(frame{
		header: header{PayloadOffset: 2}, body: []byte{1},
	}, cipherState{}, false)
	if err == nil || !strings.Contains(err.Error(), "invalid payload offset") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func FuzzReadFrame(f *testing.F) {
	f.Add([]byte{0xf0, 0xde, 0xbc, 0x0a})
	f.Add(make([]byte, 24))
	limits, _ := (Limits{MaxBody: 4096, MaxExtension: 1024}).normalized()
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = readFrame(bytes.NewReader(data), limits)
	})
}
