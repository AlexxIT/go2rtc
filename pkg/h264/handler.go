package h264

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func CreateHandler(codec *core.Codec) core.CodecHandler {
	return core.NewCodecHandler(
		codec,
		IsKeyframe,
		RTPDepay,
		RepairAVCC,
		&Payloader{IsAVC: true},
	)
}

func init() {
	core.RegisterCodecHandler(core.CodecH264, CreateHandler)
}
