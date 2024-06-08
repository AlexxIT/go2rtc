package opus

import (
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func Log(handler core.HandlerFunc) core.HandlerFunc {
	var ts uint32

	return func(pkt *rtp.Packet) {
		if ts == 0 {
			ts = pkt.Timestamp
		}

		toc := pkt.Payload[0]
		//config := toc >> 3
		code := toc & 0b11

		frame := parseFrameSize(toc)
		rate := parseSampleRate(toc)

		log.Printf(
			"[RTP/OPUS] frame=%s rate=%5d code=%d size=%6d ts=%10d dt=%5d pt=%2d ssrc=%d seq=%d mark=%t",
			frame, rate, code, len(pkt.Payload), pkt.Timestamp, pkt.Timestamp-ts, pkt.PayloadType, pkt.SSRC, pkt.SequenceNumber, pkt.Marker,
		)

		ts = pkt.Timestamp

		handler(pkt)
	}
}

func parseFrameSize(toc byte) time.Duration {
	switch toc >> 3 {
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

func parseSampleRate(toc byte) uint16 {
	switch toc >> 3 {
	case 0, 1, 2, 3, 16, 17, 18, 19:
		return 8000
	case 4, 5, 6, 7:
		return 12000
	case 8, 9, 10, 11, 20, 21, 22, 23:
		return 16000
	case 12, 13, 24, 25, 26, 27:
		return 24000
	case 14, 15, 28, 29, 30, 31:
		return 48000
	}
	return 0
}
