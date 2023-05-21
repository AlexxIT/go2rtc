package pcm

import (
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func Resample(codec *core.Codec, sampleRate uint32, handler core.HandlerFunc) core.HandlerFunc {
	n := float32(codec.ClockRate) / float32(sampleRate)

	switch codec.Name {
	case core.CodecPCMA:
		return DownsampleByte(PCMAtoPCM, PCMtoPCMA, n, handler)
	case core.CodecPCMU:
		return DownsampleByte(PCMUtoPCM, PCMtoPCMU, n, handler)
	case core.CodecPCM:
		if n == 1 {
			return ResamplePCM(PCMtoPCMA, handler)
		}
		return DownsamplePCM(PCMtoPCMA, n, handler)
	}

	panic(core.Caller())
}

func DownsampleByte(
	toPCM func(byte) int16, fromPCM func(int16) byte, n float32, handler core.HandlerFunc,
) core.HandlerFunc {
	var sampleN, sampleSum float32
	var ts uint32

	return func(packet *rtp.Packet) {
		samples := len(packet.Payload)
		newLen := uint32((float32(samples) + sampleN) / n)

		oldSamples := packet.Payload
		newSamples := make([]byte, newLen)

		var i int
		for _, sample := range oldSamples {
			sampleSum += float32(toPCM(sample))
			if sampleN++; sampleN >= n {
				newSamples[i] = fromPCM(int16(sampleSum / n))
				i++

				sampleSum = 0
				sampleN -= n
			}
		}

		ts += newLen

		clone := *packet
		clone.Payload = newSamples
		clone.Timestamp = ts
		handler(&clone)
	}
}

func ResamplePCM(fromPCM func(int16) byte, handler core.HandlerFunc) core.HandlerFunc {
	var ts uint32

	return func(packet *rtp.Packet) {
		len1 := len(packet.Payload)
		len2 := len1 / 2

		oldSamples := packet.Payload
		newSamples := make([]byte, len2)

		var i2 int
		for i1 := 0; i1 < len1; i1 += 2 {
			sample := int16(uint16(oldSamples[i1])<<8 | uint16(oldSamples[i1+1]))
			newSamples[i2] = fromPCM(sample)
			i2++
		}

		ts += uint32(len2)

		clone := *packet
		clone.Payload = newSamples
		clone.Timestamp = ts
		handler(&clone)
	}
}

func DownsamplePCM(fromPCM func(int16) byte, n float32, handler core.HandlerFunc) core.HandlerFunc {
	var sampleN, sampleSum float32
	var ts uint32

	return func(packet *rtp.Packet) {
		samples := len(packet.Payload) / 2
		newLen := uint32((float32(samples) + sampleN) / n)

		oldSamples := packet.Payload
		newSamples := make([]byte, newLen)

		var i2 int
		for i1 := 0; i1 < len(packet.Payload); i1 += 2 {
			sampleSum += float32(int16(uint16(oldSamples[i1])<<8 | uint16(oldSamples[i1+1])))
			if sampleN++; sampleN >= n {
				newSamples[i2] = fromPCM(int16(sampleSum / n))
				i2++

				sampleSum = 0
				sampleN -= n
			}
		}

		ts += newLen

		clone := *packet
		clone.Payload = newSamples
		clone.Timestamp = ts
		handler(&clone)
	}
}
