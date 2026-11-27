// Package h264 - AVCC format related functions
package h264

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

func RepairAVCC(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	sps, pps := GetParameterSet(codec.FmtpLine)
	ps := JoinNALU(sps, pps)

	return func(packet *rtp.Packet) {
		payload := annexb.NormalizeAVCC(packet.Payload, 1)
		if len(payload) == 0 {
			return
		}

		// FLV from FFmpeg can start with a SEI NAL unit.
		if NALUType(payload) == NALUTypeSEI {
			size := annexb.AVCCNALUSize(payload)
			if size == 0 || size >= len(payload) {
				return
			}
			payload = payload[size:]
		}

		prependPS := len(ps) > 0 && NALUType(payload) == NALUTypeIFrame
		if prependPS {
			payload = Join(ps, payload)
		}

		if prependPS || len(payload) != len(packet.Payload) {
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
	for len(avcc) > 0 {
		size := annexb.AVCCNALUSize(avcc)
		if size == 0 {
			break
		}
		nals = append(nals, avcc[:size])
		avcc = avcc[size:]
	}
	return nals
}

func NALUTypes(avcc []byte) []byte {
	var types []byte
	for len(avcc) > 0 {
		size := annexb.AVCCNALUSize(avcc)
		if size == 0 {
			break
		}
		types = append(types, NALUType(avcc))
		avcc = avcc[size:]
	}
	return types
}

func AVCCToCodec(avcc []byte) *core.Codec {
	buf := bytes.NewBufferString("packetization-mode=1")

	for len(avcc) > 0 {
		size := annexb.AVCCNALUSize(avcc)
		if size == 0 {
			break
		}

		if size > 4 {
			switch NALUType(avcc) {
			case NALUTypeSPS:
				if size >= 8 {
					buf.WriteString(";profile-level-id=")
					buf.WriteString(hex.EncodeToString(avcc[5:8]))
					buf.WriteString(";sprop-parameter-sets=")
					buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
				}
			case NALUTypePPS:
				buf.WriteString(",")
				buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
			}
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
