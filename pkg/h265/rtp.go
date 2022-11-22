package h265

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/codec/h265parser"
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

// SafariPay - generate Safari friendly payload for H265
func SafariPay(mtu uint16) streamer.WrapperFunc {
	sequencer := rtp.NewRandomSequencer()
	size := int(mtu - 12) // rtp.Header size

	var buffer []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			if packet.Version != h264.RTPPacketVersionAVC {
				return push(packet)
			}

			data := packet.Payload
			data[0] = 0
			data[1] = 0
			data[2] = 0
			data[3] = 1

			var start byte

			nuType := (data[4] >> 1) & 0b111111
			//fmt.Printf("[H265] nut: %2d, size: %6d, data: %16x\n", nut, len(data), data[4:20])
			switch {
			case nuType >= h265parser.NAL_UNIT_VPS && nuType <= h265parser.NAL_UNIT_PPS:
				buffer = append(buffer, data...)
				return nil
			case nuType >= h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP && nuType <= h265parser.NAL_UNIT_CODED_SLICE_CRA:
				buffer = append([]byte{3}, buffer...)
				data = append(buffer, data...)
				start = 1
			default:
				data = append([]byte{2}, data...)
				start = 0
			}

			for len(data) > size {
				clone := rtp.Packet{
					Header: rtp.Header{
						Version:        2,
						Marker:         false,
						SequenceNumber: sequencer.NextSequenceNumber(),
						Timestamp:      packet.Timestamp,
					},
					Payload: data[:size],
				}
				if err := push(&clone); err != nil {
					return err
				}

				data = append([]byte{start}, data[size:]...)
			}

			clone := rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true,
					SequenceNumber: sequencer.NextSequenceNumber(),
					Timestamp:      packet.Timestamp,
				},
				Payload: data,
			}
			return push(&clone)
		}
	}
}
