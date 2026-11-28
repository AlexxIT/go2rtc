package wyze

import "testing"

func TestParsePTZPosition(t *testing.T) {
	position, err := parsePTZPosition([]byte{
		0x78, 0x56, 0x34, 0x12,
		0x2A,
		0xCE, 0xFF,
	})
	if err != nil {
		t.Fatal(err)
	}
	if position.ControlField != 0x12345678 || position.X != -50 || position.Y != 42 {
		t.Fatalf("position = %+v", position)
	}
}

func TestParsePTZPositionShort(t *testing.T) {
	if _, err := parsePTZPosition(make([]byte, 6)); err == nil {
		t.Fatal("expected short-response error")
	}
}
