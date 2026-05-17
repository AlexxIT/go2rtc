package main

import (
	"fmt"
)

func main() {
	// A valid ADTS header for 16000Hz mono might be:
	// FF F1 50 80 ...
	// Wait, config 1408 means:
	// Audio Object Type 2 (AAC-LC)
	// Sample Rate Index 8 (16000)
	// Channel Config 1 (Mono)
	
	// A typical ADTS header length 7 bytes.
	// b[3] = 0x80 means:
	// b[3] & 3 = 0.
	
	// Let's create an ADTS header with frame length = 60
	// 60 = 0b00000000111100
	// b[3] (last 2 bits) = 00
	// b[4] = 00000111 = 0x07
	// b[5] (first 3 bits) = 100
	
	payload := []byte{0xFF, 0xF1, 0x50, 0x00, 0x07, 0x80, 0x00}
	
	frameLen := (int(payload[3]&3) << 11) | (int(payload[4]) << 3) | (int(payload[5]) >> 5)
	fmt.Printf("frameLen = %d\n", frameLen)
}
