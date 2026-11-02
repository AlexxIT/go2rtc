package h264

import "testing"

func TestGetFmtpLineMalformed(t *testing.T) {
	for _, avc := range [][]byte{
		nil,
		{0, 0},
		{0, 0, 0, 9, 0x67, 1, 2},             // SPS size beyond data
		{0, 0, 0, 2, 0x68, 1},                // valid PPS
		{0, 0, 0, 9, 0x68, 1},                // PPS size beyond data
		{0, 0, 0, 1, 0x67},                   // SPS too short for profile
		{0, 0, 0, 1, 0x65, 0, 0},             // trailing chunk shorter than 4 bytes
		{0xFF, 0xFF, 0xFF, 0xFF, 0x67, 1, 2}, // huge size
	} {
		_ = GetFmtpLine(avc) // must not panic
	}
}
