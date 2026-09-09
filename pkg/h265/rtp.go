package h265

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

func RTPDepay(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	vps, sps, pps := GetParameterSet(codec.FmtpLine)
	ps := h264.JoinNALU(vps, sps, pps)

	buf := make([]byte, 0, 512*1024) // 512K
	var nuStart int
	var seqNum uint16
	var timestamp uint32
	var seen, fragmented, waitKeyframe bool
	reset := func() {
		buf = buf[:0]
		fragmented = false
		waitKeyframe = true
	}

	return func(packet *rtp.Packet) {
		if seen && (packet.SequenceNumber-seqNum != 1 || (fragmented && packet.Timestamp != timestamp)) {
			// Lost fragments invalidate dependent pictures until a complete keyframe arrives.
			reset()
		}
		seen, seqNum, timestamp = true, packet.SequenceNumber, packet.Timestamp
		data := packet.Payload
		if len(data) < 3 {
			reset()
			return
		}

		nuType := (data[0] >> 1) & 0x3F
		//log.Printf("[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, %v", track.Codec.Name, nuType, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

		// Fix for RtspServer https://github.com/AlexxIT/go2rtc/issues/244
		if packet.Marker && len(data) < h264.PSMaxSize {
			switch nuType {
			case NALUTypeVPS, NALUTypeSPS, NALUTypePPS:
				packet.Marker = false
			case NALUTypePrefixSEI, NALUTypeSuffixSEI:
				return
			}
		}

		if nuType == NALUTypeFU {
			switch data[2] >> 6 {
			case 0b10: // begin
				if fragmented {
					reset()
				}
				fragmented = true
				nuType = data[2] & 0x3F

				// push PS data before keyframe
				if len(buf) == 0 && nuType >= 19 && nuType <= 21 {
					buf = append(buf, ps...)
				}

				nuStart = len(buf)
				buf = append(buf, 0, 0, 0, 0) // NAL unit size
				buf = append(buf, (data[0]&0x81)|(nuType<<1), data[1])
				buf = append(buf, data[3:]...)
				return
			case 0b00: // continue
				if !fragmented {
					reset()
					return
				}

				buf = append(buf, data[3:]...)
				return
			case 0b01: // end
				if !fragmented {
					reset()
					return
				}
				fragmented = false
				buf = append(buf, data[3:]...)
				binary.BigEndian.PutUint32(buf[nuStart:], uint32(len(buf)-nuStart-4))
			case 0b11: // wrong RFC 7798 realisation from OpenIPC project
				if fragmented {
					reset()
				}
				// A non-fragmented NAL unit MUST NOT be transmitted in one FU; i.e.,
				// the Start bit and End bit must not both be set to 1 in the same FU
				// header.
				nuType = data[2] & 0x3F
				buf = binary.BigEndian.AppendUint32(buf, uint32(len(data))-1) // NAL unit size
				buf = append(buf, (data[0]&0x81)|(nuType<<1), data[1])
				buf = append(buf, data[3:]...)
			}
		} else {
			if fragmented {
				reset()
			}
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(data))) // NAL unit size
			buf = append(buf, data...)
		}

		// collect all NAL Units for Access Unit
		if !packet.Marker {
			return
		}

		if waitKeyframe {
			if !IsKeyframe(buf) {
				buf = buf[:0]
				return
			}
			waitKeyframe = false
		}

		//log.Printf("[HEVC] %v, len: %d", Types(buf), len(buf))

		clone := *packet
		clone.Version = h264.RTPPacketVersionAVC
		clone.Payload = buf

		buf = buf[:0]

		handler(&clone)
	}
}

func RTPPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	if mtu == 0 {
		mtu = 1472
	}

	payloader := &Payloader{}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != h264.RTPPacketVersionAVC {
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

// SafariPay - generate Safari friendly payload for H265
// https://github.com/AlexxIT/Blog/issues/5
func SafariPay(mtu uint16, handler core.HandlerFunc) core.HandlerFunc {
	sequencer := rtp.NewRandomSequencer()
	size := int(mtu - 12) // rtp.Header size

	return func(packet *rtp.Packet) {
		if packet.Version != h264.RTPPacketVersionAVC {
			handler(packet)
			return
		}

		// protect original packets from modification
		au := make([]byte, len(packet.Payload))
		copy(au, packet.Payload)

		var start byte

		for i := 0; i < len(au); {
			size := int(binary.BigEndian.Uint32(au[i:])) + 4

			// convert AVC to Annex-B
			au[i] = 0
			au[i+1] = 0
			au[i+2] = 0
			au[i+3] = 1

			switch NALUType(au[i:]) {
			case NALUTypeIFrame, NALUTypeIFrame2, NALUTypeIFrame3:
				start = 3
			default:
				if start == 0 {
					start = 2
				}
			}

			i += size
		}

		// rtp.Packet payload
		b := make([]byte, 1, size)
		size-- // minus header byte

		for au != nil {
			b[0] = start

			if start > 1 {
				start -= 2
			}

			if len(au) > size {
				b = append(b, au[:size]...)
				au = au[size:]
			} else {
				b = append(b, au...)
				au = nil
			}

			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         au == nil,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: b,
			}
			handler(&clone)

			b = b[:1] // clear buffer
		}
	}
}
