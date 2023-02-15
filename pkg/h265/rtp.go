package h265

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	//vps, sps, pps := GetParameterSet(track.Codec.FmtpLine)
	//ps := h264.EncodeAVC(vps, sps, pps)

	buf := make([]byte, 0, 512*1024) // 512K
	var nuStart int

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			data := packet.Payload
			nuType := (data[0] >> 1) & 0x3F
			//log.Printf("[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d, %v", track.Codec.Name, nuType, len(packet.Payload), packet.Timestamp, packet.PayloadType, packet.SSRC, packet.SequenceNumber, packet.Marker)

			// Fix for RtspServer https://github.com/AlexxIT/go2rtc/issues/244
			if packet.Marker && len(data) < h264.PSMaxSize {
				switch nuType {
				case NALUTypeVPS, NALUTypeSPS, NALUTypePPS:
					packet.Marker = false
				case NALUTypePrefixSEI, NALUTypeSuffixSEI:
					return nil
				}
			}

			if nuType == NALUTypeFU {
				switch data[2] >> 6 {
				case 2: // begin
					nuType = data[2] & 0x3F

					// push PS data before keyframe
					//if len(buf) == 0 && nuType >= 19 && nuType <= 21 {
					//	buf = append(buf, ps...)
					//}

					nuStart = len(buf)
					buf = append(buf, 0, 0, 0, 0) // NAL unit size
					buf = append(buf, (data[0]&0x81)|(nuType<<1), data[1])
					buf = append(buf, data[3:]...)
					return nil
				case 0: // continue
					buf = append(buf, data[3:]...)
					return nil
				case 1: // end
					buf = append(buf, data[3:]...)
					binary.BigEndian.PutUint32(buf[nuStart:], uint32(len(buf)-nuStart-4))
				}
			} else {
				nuStart = len(buf)
				buf = append(buf, 0, 0, 0, 0) // NAL unit size
				buf = append(buf, data...)
				binary.BigEndian.PutUint32(buf[nuStart:], uint32(len(data)))
			}

			// collect all NAL Units for Access Unit
			if !packet.Marker {
				return nil
			}

			//log.Printf("[HEVC] %v, len: %d", Types(buf), len(buf))

			clone := *packet
			clone.Version = h264.RTPPacketVersionAVC
			clone.Payload = buf

			buf = buf[:0]

			return push(&clone)
		}
	}
}

func RTPPay(mtu uint16) streamer.WrapperFunc {
	payloader := &Payloader{}
	sequencer := rtp.NewRandomSequencer()
	mtu -= 12 // rtp.Header size

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return push(packet)
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
				if err := push(&clone); err != nil {
					return err
				}
			}

			return nil
		}
	}
}

// SafariPay - generate Safari friendly payload for H265
// https://github.com/AlexxIT/Blog/issues/5
func SafariPay(mtu uint16) streamer.WrapperFunc {
	sequencer := rtp.NewRandomSequencer()
	size := int(mtu - 12) // rtp.Header size

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return push(packet)
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
				if err := push(&clone); err != nil {
					return err
				}

				b = b[:1] // clear buffer
			}

			return nil
		}
	}
}
