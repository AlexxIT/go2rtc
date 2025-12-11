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
