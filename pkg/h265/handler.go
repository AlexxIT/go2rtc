package h265

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
)

func CreateHandler(codec *core.Codec) core.CodecHandler {
	return core.NewCodecHandler(
		codec,
		IsKeyframe,
		RTPDepay,
		RepairAVCC,
		&Payloader{},
	)
}

func init() {
	core.RegisterCodecHandler(core.CodecH265, CreateHandler)
}
