package tutk

import (
	"encoding/binary"
	"testing"
)

func TestSession16RejectsFragmentWithPayloadPastBuffer(t *testing.T) {
	s := NewSession16(nil, make([]byte, 8))
	s.waitFSeq = 1
	s.waitCSeq = 1
	s.waitSize = 2048
	s.waitData = make([]byte, 2048)

	cmd := make([]byte, cmdHdrSize)
	cmd[0] = 0x01
	cmd[1] = 0x03
	binary.LittleEndian.PutUint16(cmd[4:], 1)
	binary.LittleEndian.PutUint32(cmd[8:], 2220)
	binary.LittleEndian.PutUint16(cmd[12:], 1)

	if got := s.SessionRead(0, cmd); got != msgMediaLost {
		t.Fatalf("SessionRead() = %d, want msgMediaLost (%d)", got, msgMediaLost)
	}
}

func TestSession16RejectsTruncatedMediaHeaders(t *testing.T) {
	s := NewSession16(nil, make([]byte, 8))
	if got := s.SessionRead(0, []byte{0x01, 0x03}); got != msgMediaLost {
		t.Fatalf("SessionRead() = %d, want msgMediaLost (%d)", got, msgMediaLost)
	}

	cmd := make([]byte, cmdHdrSize)
	cmd[0] = 0x01
	cmd[1] = 0x04
	binary.LittleEndian.PutUint16(cmd[14:], 1)
	if got := s.SessionRead(0, cmd); got != msgMediaLost {
		t.Fatalf("SessionRead() = %d, want msgMediaLost (%d)", got, msgMediaLost)
	}
}
