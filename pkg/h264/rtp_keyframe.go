package h264

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

// KeyframeTest returns the keyframe test for a receiver's packets: RTP payloads
// for an RTP codec, AVCC access units otherwise (core.Receiver.KeepGOP).
func KeyframeTest(codec *core.Codec) func(*core.Packet) bool {
	if codec.IsRTP() {
		return IsKeyframeRTP
	}
	return func(packet *core.Packet) bool {
		return len(packet.Payload) > 4 && IsKeyframe(packet.Payload)
	}
}

// IsKeyframeRTP reports whether an RTP packet starts or carries a keyframe:
// an SPS, or the first fragment of an IDR slice, alone or aggregated. A GOP
// cache restarts at such a packet, so a consumer that joins mid-stream starts
// with everything it needs to decode (RFC 6184 payload).
func IsKeyframeRTP(packet *core.Packet) bool {
	b := packet.Payload
	if len(b) < 2 {
		return false
	}

	switch t := b[0] & 0x1F; t {
	case NALUTypeSPS, NALUTypeIFrame:
		return true
	case stapaNALUType:
		for b = b[1:]; len(b) > 2; {
			size := int(binary.BigEndian.Uint16(b))
			if size == 0 || 2+size > len(b) {
				return false
			}
			if t := b[2] & 0x1F; t == NALUTypeSPS || t == NALUTypeIFrame {
				return true
			}
			b = b[2+size:]
		}
	case fuaNALUType:
		start := b[1]&0x80 != 0
		return start && b[1]&0x1F == NALUTypeIFrame
	}
	return false
}
