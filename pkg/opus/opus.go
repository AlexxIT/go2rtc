package opus

import (
	"time"
)

type Header struct {
	Mode       string
	SampleRate uint16
	FrameSize  time.Duration
	Channels   byte
	Frames     byte
}

func UnmarshalHeader(b []byte) *Header {
	// https://datatracker.ietf.org/doc/html/rfc6716#section-3.1
	b0 := b[0]
	config := b0 >> 3
	return &Header{
		Mode:       parseMode(config),
		SampleRate: parseSampleRate(config),
		FrameSize:  parseFrameSize(config),
		Channels:   parseChannels(b0 >> 2 & 0b1),
		Frames:     parseFrames(b0 & 0b11),
	}
}

func parseMode(config byte) string {
	if config <= 11 {
		return "silk"
	}
	if config <= 15 {
		return "hybrid"
	}
	return "celt"
}

func parseSampleRate(config byte) uint16 {
	switch config {
	case 0, 1, 2, 3, 16, 17, 18, 19:
		return 8000 // NB (narrowband)
	case 4, 5, 6, 7:
		return 12000 // MB (medium-band)
	case 8, 9, 10, 11, 20, 21, 22, 23:
		return 16000 // WB (wideband)
	case 12, 13, 24, 25, 26, 27:
		return 24000 // SWB (super-wideband)
	case 14, 15, 28, 29, 30, 31:
		return 48000 // FB (fullband)
	}
	return 0
}

func parseFrameSize(config byte) time.Duration {
	switch config {
	case 0, 4, 8, 12, 14, 18, 22, 26, 30:
		return 10_000_000
	case 1, 5, 9, 13, 15, 19, 23, 27, 31:
		return 20_000_000
	case 2, 6, 10:
		return 40_000_000
	case 3, 7, 11:
		return 60_000_000
	case 16, 20, 24, 28:
		return 2_500_000
	case 17, 21, 25, 29:
		return 5_000_000
	}
	return 0
}

func parseChannels(s byte) byte {
	if s == 1 {
		return 2
	}
	return 1
}

func parseFrames(c byte) byte {
	switch c {
	case 0:
		return 1
	case 1, 2:
		return 2
	}
	return 0xFF
}

func JoinFrames(b1, b2 []byte) []byte {
	// can't join
	if b1[0]&0b11 != 0 || b2[0]&0b11 != 0 {
		return append(b1, b2...)
	}

	size1, size2 := len(b1)-1, len(b2)-1

	// join same sizes
	if size1 == size2 {
		b := make([]byte, 1+size1+size2)
		copy(b, b1)
		copy(b[1+size1:], b2[1:])
		b[0] |= 0b01
		return b
	}

	b := make([]byte, 1, 3+size1+size2)
	b[0] = b1[0] | 0b10
	if size1 >= 252 {
		b0 := 252 + byte(size1)&0b11
		b = append(b, b0, byte(size1/4)-b0)
	} else {
		b = append(b, byte(size1))
	}

	b = append(b, b1[1:]...)
	b = append(b, b2[1:]...)
	return b
}
