// Package h265 - AVCC format related functions
package h265

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h264/annexb"
	"github.com/pion/rtp"
)

func RepairAVCC(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	vps, sps, pps := GetParameterSet(codec.FmtpLine)
	ps := h264.JoinNALU(vps, sps, pps)

	return func(packet *rtp.Packet) {
		payload := annexb.NormalizeAVCC(packet.Payload, h265NaluHeaderSize)
		if len(payload) == 0 {
			return
		}

		typ := NALUType(payload)
		prependPS := len(ps) > 0 && (typ == NALUTypeIFrame || typ == NALUTypeIFrame2 || typ == NALUTypeIFrame3)
		if prependPS {
			payload = h264.Join(ps, payload)
		}

		if prependPS || len(payload) != len(packet.Payload) {
			clone := *packet
			clone.Payload = payload
			packet = &clone
		}
		handler(packet)
	}
}

func AVCCToCodec(avcc []byte) *core.Codec {
	buf := bytes.NewBufferString("profile-id=1")

	for {
		size := 4 + int(binary.BigEndian.Uint32(avcc))

		switch NALUType(avcc) {
		case NALUTypeVPS:
			buf.WriteString(";sprop-vps=")
			buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
		case NALUTypeSPS:
			buf.WriteString(";sprop-sps=")
			buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
		case NALUTypePPS:
			buf.WriteString(";sprop-pps=")
			buf.WriteString(base64.StdEncoding.EncodeToString(avcc[4:size]))
		}

		if size < len(avcc) {
			avcc = avcc[size:]
		} else {
			break
		}
	}

	return &core.Codec{
		Name:        core.CodecH265,
		ClockRate:   90000,
		FmtpLine:    buf.String(),
		PayloadType: core.PayloadTypeRAW,
	}
}
