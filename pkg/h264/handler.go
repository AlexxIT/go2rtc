package h264

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func CreateHandler(codec *core.Codec) core.CodecHandler {
	// sps, pps := GetParameterSet(codec.FmtpLine)

	// var repairFunc func([]byte) []byte
	// if sps != nil && pps != nil {
	// 	ps := JoinNALU(sps, pps)
	// 	repairFunc = func(payload []byte) []byte {
	// 		if IsKeyframe(payload) && !ContainsParameterSets(payload) {
	// 			return Join(ps, payload)
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
		&Payloader{IsAVC: true},
	)
}

func init() {
	core.RegisterCodecHandler(core.CodecH264, CreateHandler)
}
