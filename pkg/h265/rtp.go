package h265

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/deepch/vdk/codec/h265parser"
	"github.com/pion/rtp"
)

func RTPDepay(track *streamer.Track) streamer.WrapperFunc {
	var buffer []byte

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) error {
			naluType := (packet.Payload[0] >> 1) & 0x3f
			//fmt.Printf(
			//	"[RTP] codec: %s, nalu: %2d, size: %6d, ts: %10d, pt: %2d, ssrc: %d, seq: %d\n",
			//	track.Codec.Name, naluType, len(packet.Payload), packet.Timestamp,
			//	packet.PayloadType, packet.SSRC, packet.SequenceNumber,
			//)

			switch naluType {
			case h265parser.NAL_UNIT_CODED_SLICE_TRAIL_R:
			case h265parser.NAL_UNIT_VPS:
			case h265parser.NAL_UNIT_SPS:
			case h265parser.NAL_UNIT_PPS:
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
			default:
				//panic("not implemented")
			}

			size := make([]byte, 4)
			binary.BigEndian.PutUint32(size, uint32(len(packet.Payload)))

			clone := *packet
			clone.Version = h264.RTPPacketVersionAVC
			clone.Payload = append(size, packet.Payload...)

			return push(&clone)
		}
	}
}
