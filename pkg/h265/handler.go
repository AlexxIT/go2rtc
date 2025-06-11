package h265

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func CreateHandler(codec *core.Codec) core.CodecHandler {
	// vps, sps, pps := GetParameterSet(codec.FmtpLine)

	// var repairFunc func([]byte) []byte
	// if vps != nil && sps != nil && pps != nil {
	// 	ps := h264.JoinNALU(vps, sps, pps)
	// 	repairFunc = func(payload []byte) []byte {
	// 		if IsKeyframe(payload) && !ContainsParameterSets(payload) {
	// 			return h264.Join(ps, payload)
	// 		}
	// 		return payload
	// 	}
	// }

	return core.NewCodecHandler(
		codec,
		IsKeyframe,
		// repairFunc,
		RTPDepay,
		RepairAVCC,
		&Payloader{},
	)
}

func init() {
	core.RegisterCodecHandler(core.CodecH265, CreateHandler)
}
