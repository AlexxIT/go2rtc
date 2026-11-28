package h265

import (
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

func RTPDepay(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	// Cache parameter sets to prepend them to keyframes.
	vps, sps, pps := GetParameterSet(codec.FmtpLine)
	ps := h264.JoinNALU(vps, sps, pps)

	// Keep the current access unit and active FU start offset between packets.
	buf := make([]byte, 0, 512*1024) // 512K
	nuStart := -1
	var seqNum uint16

	return func(packet *rtp.Packet) {
		data := packet.Payload
		// Ignore short payloads and clear any incomplete access unit.
		if len(data) < 3 {
			buf = buf[:0]
			nuStart = -1
			return
		}

		nuType := (data[0] >> 1) & 0x3F

		// Drop buffered data when RTP sequence numbers are discontinuous.
		if len(buf) > 0 && packet.SequenceNumber-seqNum != 1 {
			//log.Printf("broken H265 sequence")
			buf = buf[:0]
			nuStart = -1
			return
		}

		// Update the sequence number even for packets skipped below.
		seqNum = packet.SequenceNumber

		if nuType == NALUTypeFU && len(data) < 4 {
			buf = buf[:0]
			nuStart = -1
			return
		}

		// Require an unfinished FU to continue or end before another NALU starts.
		if nuStart >= 0 && (nuType != NALUTypeFU || data[2]&0x80 != 0) {
			buf = buf[:0]
			nuStart = -1
			return
		}

		marker := packet.Marker

		// Fix for RtspServer https://github.com/AlexxIT/go2rtc/issues/244
		if marker && len(data) < h264.PSMaxSize {
			switch nuType {
			case NALUTypeVPS, NALUTypeSPS, NALUTypePPS:
				// Keep parameter sets for the next access unit.
				marker = false
			case NALUTypePrefixSEI, NALUTypeSuffixSEI:
				// Skip standalone SEI before picture data, but keep SEI
				// that completes an access unit (e.g. Haima cameras).
				if !hasVCL(buf) {
					return
				}
			}
		}

		if nuType == NALUTypeFU {
			switch data[2] >> 6 {
			case 0b10: // begin
				// Start an AVCC NALU and prepend parameter sets before an IDR.
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
				// Append this fragment to the active FU.
				if nuStart < 0 {
					buf = buf[:0]
					return
				}

				buf = append(buf, data[3:]...)
				return
			case 0b01: // end
				// Append the final fragment and fill in the NALU length.
				if nuStart < 0 || nuStart+4 > len(buf) {
					buf = buf[:0]
					nuStart = -1
					return
				}

				buf = append(buf, data[3:]...)
				binary.BigEndian.PutUint32(buf[nuStart:], uint32(len(buf)-nuStart-4))
				nuStart = -1
			case 0b11: // wrong RFC 7798 realization from OpenIPC project
				// A non-fragmented NAL unit MUST NOT be transmitted in one FU; i.e.,
				// the Start bit and End bit must not both be set to 1 in the same FU
				// header.
				// Recover peers that send a whole NALU in a single FU packet.
				nuType = data[2] & 0x3F
				buf = binary.BigEndian.AppendUint32(buf, uint32(len(data))-1) // NAL unit size
				buf = append(buf, (data[0]&0x81)|(nuType<<1), data[1])
				buf = append(buf, data[3:]...)
			}
		} else if nuType == h265NaluAggregationPacketType {
			// RFC 7798 4.4.2. Aggregation Packets (APs).
			// Skip the payload header and unpack each length-prefixed NALU.
			// DONL/DOND fields are not supported, same as for FU.
			data = data[2:]

			for len(data) > 0 {
				if len(data) < 2 {
					buf = buf[:0]
					return
				}

				size := int(binary.BigEndian.Uint16(data))
				data = data[2:]

				// Drop the buffered access unit if the AP is truncated
				// or contains a NALU smaller than its header.
				if size < h265NaluHeaderSize || size > len(data) {
					buf = buf[:0]
					return
				}

				buf = binary.BigEndian.AppendUint32(buf, uint32(size))
				buf = append(buf, data[:size]...)
				data = data[size:]
			}
		} else {
			// Store a complete NALU with its AVCC length prefix.
			buf = binary.BigEndian.AppendUint32(buf, uint32(len(data))) // NAL unit size
			buf = append(buf, data...)
		}

		// Wait for the final packet before emitting the access unit.
		if !marker {
			return
		}

		//log.Printf("[HEVC] %v, len: %d", Types(buf), len(buf))

		// Preserve packet metadata and replace its payload with the AVCC access unit.
		clone := *packet
		clone.Version = h264.RTPPacketVersionAVC
		clone.Payload = buf

		buf = buf[:0]

		handler(&clone)
	}
}

// hasVCL reports whether the access unit contains picture data.
func hasVCL(avcc []byte) bool {
	for len(avcc) > 0 {
		size := annexb.AVCCNALUSize(avcc)
		if size == 0 {
			return false
		}

		// VCL NAL unit types are 0..31.
		if NALUType(avcc[:size]) < 32 {
			return true
		}

		avcc = avcc[size:]
	}

	return false
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
		au := annexb.NormalizeAVCC(packet.Payload, h265NaluHeaderSize)
		if len(au) == 0 {
			return
		}
		au = append([]byte(nil), au...)

		var start byte

		for i := 0; i < len(au); {
			size := annexb.AVCCNALUSize(au[i:])
			if size == 0 {
				return
			}

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
