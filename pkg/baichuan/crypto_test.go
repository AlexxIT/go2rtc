package baichuan

import (
	"bytes"
	"testing"
)

func TestMD5Modern(t *testing.T) {
	t.Parallel()

	if got, want := MD5Modern("admin"), "21232F297A57A5A743894A0E4A801FC"; got != want {
		t.Fatalf("MD5Modern() = %q, want %q", got, want)
	}
}

func TestBCXORRoundTrip(t *testing.T) {
	t.Parallel()

	plain := []byte("<?xml version=\"1.0\"?><body><nonce>abc</nonce></body>")
	encrypted := BCXOR(7, plain)
	decrypted := BCXOR(7, encrypted)

	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("BCXOR roundtrip mismatch: got %q want %q", decrypted, plain)
	}
}

func TestUDPXORRoundTrip(t *testing.T) {
	t.Parallel()

	plain := []byte("<?xml version=\"1.0\"?><P2P><C2D_C/></P2P>")
	encrypted := UDPXOR(87, plain)
	decrypted := UDPXOR(87, encrypted)

	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("UDPXOR roundtrip mismatch: got %q want %q", decrypted, plain)
	}
}
