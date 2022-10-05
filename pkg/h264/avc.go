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

func EncodeAVC(nals ...[]byte) (avc []byte) {
	n := 4 * len(nals)
	for _, nal := range nals {
		n += len(nal)
	}

	avc = make([]byte, n)

	var i int
	for _, nal := range nals {
		binary.BigEndian.PutUint32(avc[i:], uint32(len(nal)))
		i += 4 + copy(avc[i+4:], nal)
	}

	return
}

func RepairAVC(track *streamer.Track) streamer.WrapperFunc {
	sps, pps := GetParameterSet(track.Codec.FmtpLine)
	ps := EncodeAVC(sps, pps)

	return func(push streamer.WriterFunc) streamer.WriterFunc {
		return func(packet *rtp.Packet) (err error) {
			if NALUType(packet.Payload) == NALUTypeIFrame {
				packet.Payload = Join(ps, packet.Payload)
			}
			return push(packet)
		}
	}
}

func SplitAVC(data []byte) [][]byte {
	var nals [][]byte
	for {
		// get AVC length
		size := int(binary.BigEndian.Uint32(data)) + 4

		// check if multiple items in one packet
		if size < len(data) {
			nals = append(nals, data[:size])
			data = data[size:]
		} else {
			nals = append(nals, data)
			break
		}
	}
	return nals
}

func Types(data []byte) []byte {
	var types []byte
	for {
		types = append(types, NALUType(data))

		size := 4 + int(binary.BigEndian.Uint32(data))
		if size < len(data) {
			data = data[size:]
		} else {
			break
		}
	}
	return types
}
