package debug

import (
	"fmt"
	"time"

	"github.com/pion/rtp"
)

func Logger(include func(packet *rtp.Packet) bool) func(packet *rtp.Packet) {
	var lastTime = time.Now()
	var lastTS uint32

	var secCnt int
	var secSize int
	var secTS uint32
	var secTime time.Time

	return func(packet *rtp.Packet) {
		if include != nil && !include(packet) {
			return
		}

		now := time.Now()

		fmt.Printf(
			"%s: size=%6d ts=%10d type=%2d ssrc=%d seq=%5d mark=%t dts=%4d dtime=%3dms\n",
			now.Format("15:04:05.000"),
			len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker,
			packet.Timestamp-lastTS, now.Sub(lastTime).Milliseconds(),
		)

		lastTS = packet.Timestamp
		lastTime = now

		if secTS == 0 {
			secTS = lastTS
			secTime = now
			return
		}

		if dt := now.Sub(secTime); dt > time.Second {
			fmt.Printf(
				"%s: size=%6d cnt=%d dts=%d dtime=%3dms\n",
				now.Format("15:04:05.000"),
				secSize, secCnt, lastTS-secTS, dt.Milliseconds(),
			)

			secCnt = 0
			secSize = 0
			secTS = lastTS
			secTime = now
		}

		secCnt++
		secSize += len(packet.Payload)
	}
}
