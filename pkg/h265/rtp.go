package h265

import (
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/pion/rtp"
)

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	vps, sps, pps := GetParameterSet(track.Codec.FmtpLine)

	var buffer []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			nut := (packet.Payload[0] >> 1) & 0x3f
			//fmt.Printf(
			//	"[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d\n",
			//	track.Codec.Name, nut, len(packet.Payload), packet.Timestamp,
			//	packet.PayloadType, packet.SSRC, packet.SequenceNumber,
			//)

			switch nut {
			case h265parser.NAL_UNIT_UNSPECIFIED_49:
				data := packet.Payload
				switch data[2] >> 6 {
				case 2: // begin
					buffer = []byte{
						(data[0] & 0x81) | (data[2] & 0x3f << 1), data[1],
					}
					buffer = append(buffer, data[3:]...)
					return nil
				case 0: // continue
					buffer = append(buffer, data[3:]...)
					return nil
				case 1: // end
					packet.Payload = append(buffer, data[3:]...)
				}
			case h265parser.NAL_UNIT_VPS:
				vps = packet.Payload
				return nil
			case h265parser.NAL_UNIT_SPS:
				sps = packet.Payload
				return nil
			case h265parser.NAL_UNIT_PPS:
				pps = packet.Payload
				return nil
			default:
				//panic("not implemented")
			}

			var clone rtp.Packet

			nut = (packet.Payload[0] >> 1) & 0x3f
			if nut >= h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP && nut <= h265parser.NAL_UNIT_CODED_SLICE_CRA {
				clone = *packet
				clone.Version = h264.RTPPacketVersionAVC
				clone.Payload = h264.EncodeAVC(vps)
				if err := push(&clone); err != nil {
					return err
				}

				clone = *packet
				clone.Version = h264.RTPPacketVersionAVC
				clone.Payload = h264.EncodeAVC(sps)
				if err := push(&clone); err != nil {
					return err
				}

				clone = *packet
				clone.Version = h264.RTPPacketVersionAVC
				clone.Payload = h264.EncodeAVC(pps)
				if err := push(&clone); err != nil {
					return err
				}
			}

			clone = *packet
			clone.Version = h264.RTPPacketVersionAVC
			clone.Payload = h264.EncodeAVC(packet.Payload)

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

			nut := (data[4] >> 1) & 0b111111
			//fmt.Printf("[H265] nut: %2d, size: %6d, data: %16x\n", nut, len(data), data[4:20])
			switch {
			case nut >= h265parser.NAL_UNIT_VPS && nut <= h265parser.NAL_UNIT_PPS:
				buffer = append(buffer, data...)
				return nil
			case nut >= h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP && nut <= h265parser.NAL_UNIT_CODED_SLICE_CRA:
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
