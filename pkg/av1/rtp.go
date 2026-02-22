package av1

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
)

const RTPPacketVersionAV1 = 0

// RTPDepay converts AV1 RTP packets (RFC 9583) into AV1 temporal units
// in low overhead bitstream format (OBUs with obu_has_size_field=1),
// suitable for MP4/fMP4 muxing.
//
// Uses pion's AV1Depacketizer for correct handling of OBU fragmentation,
// aggregation headers, and size field conversion.
func RTPDepay(handler core.HandlerFunc) core.HandlerFunc {
	depack := &codecs.AV1Depacketizer{}
	buf := make([]byte, 0, 512*1024)

	return func(packet *rtp.Packet) {
		payload, err := depack.Unmarshal(packet.Payload)
		if len(payload) == 0 || err != nil {
			return
		}

		// Memory overflow protection
		if len(buf) > 5*1024*1024 {
			buf = buf[:0:512*1024]
		}

		// Collect OBUs for the complete temporal unit
		buf = append(buf, payload...)

		if !packet.Marker {
			return // wait for complete temporal unit
		}

		if len(buf) == 0 {
			return
		}

		// Make a copy to avoid aliasing - buf is reused across calls
		payload2 := make([]byte, len(buf))
		copy(payload2, buf)
		buf = buf[:0]

		clone := *packet
		clone.Version = RTPPacketVersionAV1
		clone.Payload = payload2
		clone.ExtensionProfile = 0 // AV1 has no B-frames, CTS always equals DTS

		handler(&clone)
	}
}

// RTPPay packetizes AV1 OBUs into RTP packets with proper MTU fragmentation.
// Input packets must have Version == RTPPacketVersionAV1 with reassembled OBUs.
func RTPPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	if mtu == 0 {
		mtu = 1472
	}

	payloader := &codecs.AV1Payloader{}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != RTPPacketVersionAV1 {
			handler(packet)
			return
		}

		payloads := payloader.Payload(mtu, packet.Payload)
		last := len(payloads) - 1
		for i, payload := range payloads {
			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         i == last,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: payload,
			}
			handler(&clone)
		}
	}
}
