// Package h264 - AVCC format related functions
package h264

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func RepairAVCC(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	sps, pps := GetParameterSet(codec.FmtpLine)
	ps := JoinNALU(sps, pps)

	return func(packet *rtp.Packet) {
		payload := packet.Payload
		if len(payload) < 5 {
			return
		}

		// FLV from FFmpeg can start with a SEI NAL unit.
		stripSEI := NALUType(payload) == NALUTypeSEI
		if stripSEI {
			size := int(binary.BigEndian.Uint32(payload))
			// On 32-bit systems, an oversized uint32 becomes negative as an int.
			// Check the remaining length before adding the 4-byte prefix.
			if size <= 0 || size > len(payload)-4 {
				return
			}
			payload = payload[4+size:]
			if len(payload) < 5 {
				return
			}
		}

		prependPS := len(ps) > 0 && NALUType(payload) == NALUTypeIFrame
		if prependPS {
			payload = Join(ps, payload)
		}

		if stripSEI || prependPS {
			clone := *packet
			clone.Payload = payload
			packet = &clone
		}
		handler(packet)
	}
}

func JoinNALU(nalus ...[]byte) (avcc []byte) {
	var i, n int

	for _, nalu := range nalus {
		if i = len(nalu); i > 0 {
			n += 4 + i
		}
	}

	avcc = make([]byte, n)

	n = 0
	for _, nal := range nalus {
		if i = len(nal); i > 0 {
			binary.BigEndian.PutUint32(avcc[n:], uint32(i))
			n += 4 + copy(avcc[n+4:], nal)
		}
	}

	return
}

func SplitNALU(avcc []byte) [][]byte {
	var nals [][]byte
	for {
		// get AVC length
		size := int(binary.BigEndian.Uint32(avcc)) + 4

		// check if multiple items in one packet
		if size < len(avcc) {
			nals = append(nals, avcc[:size])
			avcc = avcc[size:]
		} else {
			nals = append(nals, avcc)
			break
		}
	}
	return nals
}

func NALUTypes(avcc []byte) []byte {
	var types []byte
	for {
		types = append(types, NALUType(avcc))

		size := 4 + int(binary.BigEndian.Uint32(avcc))
		if size < len(avcc) {
			avcc = avcc[size:]
		} else {
			break
		}
	}
	return types
}

func AVCCToCodec(avcc []byte) *core.Codec {
	buf := bytes.NewBufferString("packetization-mode=1")

	for {
		n := len(avcc)
		if n < 4 {
			break
		}

		size := 4 + int(binary.BigEndian.Uint32(avcc))
		if n < size {
			break
		}

		switch NALUType(avcc) {
		case NALUTypeSPS:
			buf.WriteString(";profile-level-id=")
			buf.WriteString(hex.EncodeToString(avcc[5:8]))
			buf.WriteString(";sprop-parameter-sets=")
			buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
		case NALUTypePPS:
			buf.WriteString(",")
			buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
		}

		avcc = avcc[size:]
	}

	return &core.Codec{
		Name:        core.CodecH264,
		ClockRate:   90000,
		FmtpLine:    buf.String(),
		PayloadType: core.PayloadTypeRAW,
	}
}
