package wyze

import (
	"bytes"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
)

func TestBuildPTZRelative(t *testing.T) {
	frame := buildPTZRelative(-50, 30, 1)
	cmd, payload, ok := tutk.ParseHL(frame)
	if !ok {
		t.Fatal("failed to parse HL frame")
	}
	if cmd != KCmdPTZRelative {
		t.Fatalf("command = %d, want %d", cmd, KCmdPTZRelative)
	}
	want := []byte{0xCE, 0xFF, 0x1E, 0x00, 0x01}
	if !bytes.Equal(payload, want) {
		t.Fatalf("payload = % X, want % X", payload, want)
	}
}
