package baichuan

import (
	"encoding/hex"
	"testing"
)

func TestModernMD5(t *testing.T) {
	if actual := modernMD5("test"); actual != "098F6BCD4621D373CADE4E832627B4F" {
		t.Fatalf("unexpected digest: %s", actual)
	}
}

func TestBC(t *testing.T) {
	state := cipherState{mode: encryptionBC}
	encoded := testCrypt(state, 2, []byte("abc"), true)
	if actual := hex.EncodeToString(encoded); actual != "5f2b3b" {
		t.Fatalf("unexpected ciphertext: %s", actual)
	}
	if actual := string(testCrypt(state, 2, encoded, false)); actual != "abc" {
		t.Fatalf("unexpected plaintext: %s", actual)
	}
}

func TestAESKnownVector(t *testing.T) {
	state := cipherState{mode: encryptionAES}
	state.setAESKey(deriveAESKey("nonce", "password"))
	encoded := testCrypt(state, 0, []byte("camera XML"), true)
	if actual := hex.EncodeToString(encoded); actual != "db73b881b9082f4c67b5" {
		t.Fatalf("unexpected ciphertext: %s", actual)
	}
	if actual := string(testCrypt(state, 0, encoded, false)); actual != "camera XML" {
		t.Fatalf("unexpected plaintext: %s", actual)
	}
}

func testCrypt(state cipherState, channel uint8, src []byte, encrypt bool) []byte {
	dst := append([]byte(nil), src...)
	state.cryptInPlace(channel, dst, encrypt)
	return dst
}
