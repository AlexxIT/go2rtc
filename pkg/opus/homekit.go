package opus

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// Some info about this magic:
// - Apple has no respect for RFC 7587 standard and using RFC 3550 for RTP timestamps
// - Apple can request packets with 20ms duration over LAN connection and 60ms over LTE
// - FFmpeg produce packets with 20ms duration by default and only one frame per packet
// - FFmpeg should use "-min_comp 0" option, so every packet will be same duration
// - Apple doesn't care about real sample rate of track
// - Apple only cares about proper timestamp based on REQUESTED sample rate

// RepackToHAP - convert standart RTP packet with OPUS to HAP packet
// We expect that:
// - incoming packet will be 20ms duration and only one frame per packet
// - outgouing packet will be 20ms or 60ms duration
// - incoming sample rate will be any (but not very big if we needs 60ms packets for output)
// - outgouing sample rate will be 16000
// https://github.com/AlexxIT/go2rtc/issues/667
func RepackToHAP(rtpTime byte, handler core.HandlerFunc) core.HandlerFunc {
	switch rtpTime {
	case 20:
		return repackToHAP20(handler)
	case 60:
		return repackToHAP60(handler)
	}
	return handler
}

// we using only one sample rate in the pkg/hap/camera/accessory.go
const (
	timestamp20 = 16000 * 0.020
	timestamp60 = 16000 * 0.060
)

// repackToHAP20 - just fix RTP timestamp from RFC 7587 to RFC 3550
func repackToHAP20(handler core.HandlerFunc) core.HandlerFunc {
	var timestamp uint32

	return func(pkt *rtp.Packet) {
		timestamp += timestamp20

		clone := *pkt
		clone.Timestamp = timestamp
		handler(&clone)
	}
}

// repackToHAP60 - collect 20ms frames to single 60ms packet
// thanks to @civita idea https://github.com/AlexxIT/go2rtc/pull/843
func repackToHAP60(handler core.HandlerFunc) core.HandlerFunc {
	var sequence uint16
	var timestamp uint32

	var framesCount byte
	var framesSize []byte
	var framesData []byte

	return func(pkt *rtp.Packet) {
		framesData = append(framesData, pkt.Payload[1:]...)

		if framesCount++; framesCount < 3 {
			if frameSize := len(pkt.Payload) - 1; frameSize >= 252 {
				b0 := 252 + byte(frameSize)&0b11
				framesSize = append(framesSize, b0, byte(frameSize/4)-b0)
			} else {
				framesSize = append(framesSize, byte(frameSize))
			}
			return
		}

		toc := pkt.Payload[0]

		payload := make([]byte, 2, 2+len(framesSize)+len(framesData))
		payload[0] = toc | 0b11  // code 3 (multiple frames per packet)
		payload[1] = 0b1000_0011 // VBR, no padding, 3 frames
		payload = append(payload, framesSize...)
		payload = append(payload, framesData...)

		sequence++
		timestamp += timestamp60

		clone := *pkt
		clone.Payload = payload
		clone.SequenceNumber = sequence
		clone.Timestamp = timestamp
		handler(&clone)

		framesCount = 0
		framesSize = framesSize[:0]
		framesData = framesData[:0]
	}
}
