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
	var i, n int

	for _, nal := range nals {
		if i = len(nal); i > 0 {
			n += 4 + i
		}
	}

	avc = make([]byte, n)

	n = 0
	for _, nal := range nals {
		if i = len(nal); i > 0 {
			binary.BigEndian.PutUint32(avc[n:], uint32(i))
			n += 4 + copy(avc[n+4:], nal)
		}
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
