package baichuan

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
		commandLogin: maxLoginBody, commandTalkAbility: maxTalkAbilityBody,
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

func TestReadFrameHonorsConfiguredLoginBodyLimit(t *testing.T) {
	limits := testLimits(t)
	limits.MaxBody = 7338
	limits.MaxExtension = limits.MaxBody
	header := make([]byte, 20)
	binary.LittleEndian.PutUint32(header, wireMagic)
	binary.LittleEndian.PutUint32(header[4:], commandLogin)
	binary.LittleEndian.PutUint32(header[8:], 7339)
	binary.LittleEndian.PutUint16(header[18:], classLegacy)
	if _, err := readFrame(bytes.NewReader(header), limits); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("accepted login body above configured limit: %v", err)
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

func TestDecodeFrameInfersUnencryptedPreviewMedia(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    encryption
		command uint32
		ext     string
		control bool
		binary  bool
	}{
		{name: "media", command: commandPreview, binary: true},
		{name: "explicit control", command: commandPreview, ext: "<Extension><binaryData>0</binaryData></Extension>"},
		{name: "XML control", command: commandPreview, control: true},
		{name: "other command", command: commandDeviceInfo},
		{name: "BC raw magic", command: commandPreview, mode: encryptionBC},
		{name: "AES raw magic", command: commandPreview, mode: encryptionAES},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := cipherState{mode: test.mode}
			state.setAESKey(deriveAESKey("nonce", "password"))
			payload := infoFixture()
			if test.control {
				payload = []byte("<body><Preview/></body>")
			}
			want := bytes.Clone(payload)
			if !test.binary {
				want = testCrypt(state, 1, want, false)
			}
			ext := testCrypt(state, 1, []byte(test.ext), true)
			msg, err := decodeFrame(frame{
				header: header{Command: test.command, Channel: 1, PayloadOffset: uint32(len(ext))},
				body:   append(ext, payload...),
			}, state, false)
			if err != nil || msg.binary != test.binary || !bytes.Equal(msg.payload, want) {
				t.Fatalf("binary=%t, want %t; payload=%x, want %x; err=%v", msg.binary, test.binary, msg.payload, want, err)
			}
		})
	}
}

func TestDecodeFrameExplicitControlOverridesHint(t *testing.T) {
	for _, mode := range []encryption{encryptionNone, encryptionBC, encryptionAES} {
		for _, test := range []struct {
			flag         int
			hint, binary bool
		}{
			{0, false, false}, {0, true, false},
			{1, false, true}, {1, true, true},
			{7, false, false}, {7, true, true},
		} {
			t.Run(fmt.Sprintf("%d/flag=%d/hint=%t", mode, test.flag, test.hint), func(t *testing.T) {
				state := cipherState{mode: mode}
				state.setAESKey(deriveAESKey("nonce", "password"))
				ext := []byte(fmt.Sprintf("<Extension><binaryData>%d</binaryData></Extension>", test.flag))
				payload := []byte("<body><Preview/></body>")
				encoded := payload
				if !test.binary {
					encoded = testCrypt(state, 1, payload, true)
				}
				body := append(testCrypt(state, 1, ext, true), encoded...)
				msg, err := decodeFrame(frame{
					header: header{Channel: 1, PayloadOffset: uint32(len(ext))}, body: body,
				}, state, test.hint)
				if err != nil || msg.binary != test.binary || !bytes.Equal(msg.payload, payload) {
					t.Fatalf("binary=%t, want %t; payload=%q error=%v", msg.binary, test.binary, msg.payload, err)
				}
			})
		}
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
