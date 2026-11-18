package reolink

import (
	"encoding/binary"
	"math"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
)

// talkQuiet recognizes only the digital floor, including G.711 silence codes.
// PCM payload alignment is checked by Write before this scan.
func talkQuiet(payload []byte, codec string) bool {
	for i := 0; i < len(payload); i++ {
		var sample int16
		switch codec {
		case core.CodecPCMA:
			sample = pcm.PCMAtoPCM(payload[i])
		case core.CodecPCMU:
			sample = pcm.PCMUtoPCM(payload[i])
		case core.CodecPCM:
			sample = int16(binary.BigEndian.Uint16(payload[i:]))
			i++
		case core.CodecPCML:
			sample = int16(binary.LittleEndian.Uint16(payload[i:]))
			i++
		}
		if sample < -16 || sample > 16 {
			return false
		}
	}
	return true
}

// 31-tap halfband interpolation, independently derived from
// 2*sin(pi*d/2)/(pi*d) * I0(4*sqrt(1-(d/15)^2))/I0(4), d=k-15.
// I0(x) was evaluated offline as sum((x*x/4)^j/(j!)^2), j=0..29.
// The even taps are normalized to sum to one. The other phase is a unit
// center tap, giving unity DC in both phases and 15/16000 seconds delay.
var talkHalfband = [16]float64{
	-0.0037610384125486, 0.0098619747882836, -0.0200065825258646, 0.0360257783264360,
	-0.0613834132980222, 0.1047723261864593, -0.1982336791085952, 0.6327246340438517,
	0.6327246340438517, -0.1982336791085952, 0.1047723261864593, -0.0613834132980222,
	0.0360257783264360, -0.0200065825258646, 0.0098619747882836, -0.0037610384125486,
}

type talkInterpolator struct {
	history [16]int16
}

func (f *talkInterpolator) convert(src []byte) []byte {
	dst := make([]byte, len(src)*2)
	for i := 0; i < len(src); i += 2 {
		copy(f.history[1:], f.history[:15])
		f.history[0] = int16(binary.LittleEndian.Uint16(src[i:]))
		var sample float64
		for j, coefficient := range talkHalfband {
			sample += coefficient * float64(f.history[j])
		}
		sample = math.Max(-32768, math.Min(32767, math.Round(sample)))
		binary.LittleEndian.PutUint16(dst[2*i:], uint16(int16(sample)))
		binary.LittleEndian.PutUint16(dst[2*i+2:], uint16(f.history[7]))
	}
	return dst
}
