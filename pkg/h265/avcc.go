// Package h265 - AVCC format related functions
package h265

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

func RepairAVCC(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	vds, sps, pps := GetParameterSet(codec.FmtpLine)
	ps := h264.JoinNALU(vds, sps, pps)

	fmtpLineUpdated := false

	return func(packet *rtp.Packet) {
		// Update FmtpLine from first keyframe with parameter sets
		// This fixes MSE aspect ratio issues when RTSP cameras don't send VPS/SPS/PPS in DESCRIBE
		if !fmtpLineUpdated && ContainsParameterSets(packet.Payload) {
			newFmtpLine := GetFmtpLine(packet.Payload)
			if newFmtpLine != "" {
				codec.FmtpLine = newFmtpLine
				// Re-extract VPS/SPS/PPS with updated FmtpLine
				vds, sps, pps = GetParameterSet(codec.FmtpLine)
				ps = h264.JoinNALU(vds, sps, pps)
			}
			fmtpLineUpdated = true
		}

		switch NALUType(packet.Payload) {
		case NALUTypeIFrame, NALUTypeIFrame2, NALUTypeIFrame3:
			clone := *packet
			clone.Payload = h264.Join(ps, packet.Payload)
			handler(&clone)
		default:
			handler(packet)
		}
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
