package ps

import (
	"bytes"
	"testing"
)

func TestUnmarshalSPS(t *testing.T) {
	raw := []byte{0x67, 0x42, 0x00, 0x0a, 0xf8, 0x41, 0xa2}
	s := SPS{}
	if err := s.Unmarshal(raw); err != nil {
		t.Fatal(err)
	}
	raw2 := s.Marshal()
	if bytes.Compare(raw, raw2) != 0 {
		t.Fatal()
	}
}

func TestUnmarshalPPS(t *testing.T) {
	raw := []byte{0x68, 0xce, 0x38, 0x80}
	p := PPS{}
	if err := p.Unmarshal(raw); err != nil {
		t.Fatal(err)
	}
	raw2 := p.Marshal()
	if bytes.Compare(raw, raw2) != 0 {
		t.Fatal()
	}
}

func TestUnmarshalPPS2(t *testing.T) {
	raw := []byte{72, 238, 60, 128}
	p := PPS{}
	if err := p.Unmarshal(raw); err != nil {
		t.Fatal(err)
	}
	raw2 := p.Marshal()
	if bytes.Compare(raw, raw2) != 0 {
		t.Fatal()
	}
}

func TestSafari(t *testing.T) {
	// CB66, L3.1: chrome, edge, safari, android chrome
	s := EncodeProfile(0x42, 0xE0)
	t.Logf("Profile: %s, Level: %d", s, 0x1F)

	// B66, L3.1: chrome, edge
	s = EncodeProfile(0x42, 0x00)
	t.Logf("Profile: %s, Level: %d", s, 0x1F)

	// M77, L3.1: chrome, edge
	s = EncodeProfile(0x4D, 0x00)
	t.Logf("Profile: %s, Level: %d", s, 0x1F)
}
