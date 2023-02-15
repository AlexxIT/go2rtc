package h264

import (
	"bytes"
	"encoding/binary"
	"github.com/AlexxIT/go2rtc/pkg/streamer"
	"github.com/pion/rtp"
)

func AnnexB2AVC(b []byte) []byte {
	for i := 0; i < len(b); {
		if i+4 >= len(b) {
			break
		}

		size := bytes.Index(b[i+4:], []byte{0, 0, 0, 1})
		if size < 0 {
			size = len(b) - (i + 4)
		}

		binary.BigEndian.PutUint32(b[i:], uint32(size))

		i += size + 4
	}

	return b
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
