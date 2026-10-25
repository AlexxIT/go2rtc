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

	return func(packet *rtp.Packet) {
		// AVCC needs a four-byte length prefix followed by a NALU header.
		// Some cameras intermittently emit an empty/truncated video packet.
		// Dropping that packet keeps one malformed source from panicking the
		// sender goroutine and terminating the whole go2rtc process.
		if packet == nil || len(packet.Payload) < 5 {
			return
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
		n := len(avcc)
		if n < 5 {
			break
		}

		naluSize := binary.BigEndian.Uint32(avcc)
		// An H.265 NAL unit has a two-byte header. Reject zero-length,
		// one-byte, and over-declared units before inspecting their type.
		if naluSize < 2 || naluSize > uint32(n-4) {
			break
		}
		size := 4 + int(naluSize)

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

		avcc = avcc[size:]
	}

	return &core.Codec{
		Name:        core.CodecH265,
		ClockRate:   90000,
		FmtpLine:    buf.String(),
		PayloadType: core.PayloadTypeRAW,
	}
}
