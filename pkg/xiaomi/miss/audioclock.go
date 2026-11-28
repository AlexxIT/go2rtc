package miss

// audioResyncMS is well above the per-packet jitter of camera timestamps (~10ms)
const audioResyncMS = 200

// audioClock advances the audio RTP timestamp by one frame per packet, which keeps it smooth,
// but jumps to the camera timestamp once they drift apart. Without the jump every gap in the
// camera's audio (lost packets, or audio the camera skips with contiguous sequence numbers)
// puts audio further behind video for as long as the producer runs.
type audioClock struct {
	ts uint32
}

func (a *audioClock) Next(cameraMS uint64, clockRate, frame uint32) uint32 {
	if cameraMS != 0 {
		cam := TimeToRTP(cameraMS, uint64(clockRate))
		limit := int32(audioResyncMS * clockRate / 1000)
		if d := int32(cam - a.ts); d > limit || d < -limit {
			a.ts = cam
		}
	}
	ts := a.ts
	a.ts += frame
	return ts
}
