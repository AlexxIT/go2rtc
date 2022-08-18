package h264

import (
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

const PayloadTypeAVC = 255

func IsAVC(codec *streamer.Codec) bool {
	return codec.PayloadType == PayloadTypeAVC
}

func EncodeAVC(raw []byte) (avc []byte) {
	avc = make([]byte, len(raw)+4)
	binary.BigEndian.PutUint32(avc, uint32(len(raw)))
	copy(avc[4:], raw)
	return
}

func RepairAVC(track *streamer.Track) streamer.WrapperFunc {
	sps, pps := GetParameterSet(track.Codec.FmtpLine)
	sps = EncodeAVC(sps)
	pps = EncodeAVC(pps)

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) (err error) {
			naluType := NALUType(packet.Payload)
			switch naluType {
			case NALUTypeSPS:
				sps = packet.Payload
				return
			case NALUTypePPS:
				pps = packet.Payload
				return
			}

			var clone rtp.Packet

			if naluType == NALUTypeIFrame {
				clone = *packet
				clone.Payload = sps
				if err = push(&clone); err != nil {
					return
				}

				clone = *packet
				clone.Payload = pps
				if err = push(&clone); err != nil {
					return
				}
			}

			clone = *packet
			clone.Payload = packet.Payload
			return push(&clone)
		}
	}
}
