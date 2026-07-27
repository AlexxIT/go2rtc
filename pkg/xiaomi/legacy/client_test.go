package legacy

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
)

func TestIPC017DecodePayload(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	video := []byte{0, 0, 0, 1, 0x40, 1, 2, 3}
	encryptedVideo, err := crypto.Encode(video, key)
	if err != nil {
		t.Fatal(err)
	}

	client := &Client{key: key, model: ModelIPC017}

	got, err := client.decodePayload(tutk.CodecH265, encryptedVideo)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, video) {
		t.Fatalf("unexpected decoded video: %x", got)
	}

	audio := []byte{1, 2, 3, 4}
	got, err = client.decodePayload(codecXiaobaiPCMA, audio)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, audio) {
		t.Fatalf("unexpected decoded audio: %x", got)
	}
}

func TestSupportedIPC017(t *testing.T) {
	if !Supported(ModelIPC017) {
		t.Fatal("IPC017 should be supported")
	}
}
