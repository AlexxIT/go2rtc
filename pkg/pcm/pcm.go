package pcm

import (
	"math"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

func ceil(x float32) int {
	d, fract := math.Modf(float64(x))
	if fract == 0.0 {
		return int(d)
	}
	return int(d) + 1
}

func Downsample(k float32) func([]int16) []int16 {
	var sampleN, sampleSum float32

	return func(src []int16) (dst []int16) {
		var i int
		dst = make([]int16, ceil((float32(len(src))+sampleN)/k))
		for _, sample := range src {
			sampleSum += float32(sample)
			sampleN++
			if sampleN >= k {
				dst[i] = int16(sampleSum / k)
				i++

				sampleSum = 0
				sampleN -= k
			}
		}
		return
	}
}

func Upsample(k float32) func([]int16) []int16 {
	var sampleN float32

	return func(src []int16) (dst []int16) {
		var i int
		dst = make([]int16, ceil(k*float32(len(src))))
		for _, sample := range src {
			sampleN += k
			for sampleN > 0 {
				dst[i] = sample
				i++

				sampleN -= 1
			}
		}
		return
	}
}

func FlipEndian(src []byte) (dst []byte) {
	var i, j int
	n := len(src)
	dst = make([]byte, n)
	for i < n {
		x := src[i]
		i++
		dst[j] = src[i]
		j++
		i++
		dst[j] = x
		j++
	}
	return
}

func Transcode(dst, src *core.Codec) func([]byte) []byte {
	var reader func([]byte) []int16
	var writer func([]int16) []byte
	var filters []func([]int16) []int16

	switch src.Name {
	case core.CodecPCML:
		reader = func(src []byte) (dst []int16) {
			var i, j int
			n := len(src)
			dst = make([]int16, n/2)
			for i < n {
				lo := src[i]
				i++
				hi := src[i]
				i++
				dst[j] = int16(hi)<<8 | int16(lo)
				j++
			}
			return
		}
	case core.CodecPCM:
		reader = func(src []byte) (dst []int16) {
			var i, j int
			n := len(src)
			dst = make([]int16, n/2)
			for i < n {
				hi := src[i]
				i++
				lo := src[i]
				i++
				dst[j] = int16(hi)<<8 | int16(lo)
				j++
			}
			return
		}
	case core.CodecPCMU:
		reader = func(src []byte) (dst []int16) {
			var i int
			dst = make([]int16, len(src))
			for _, sample := range src {
				dst[i] = PCMUtoPCM(sample)
				i++
			}
			return
		}
	case core.CodecPCMA:
		reader = func(src []byte) (dst []int16) {
			var i int
			dst = make([]int16, len(src))
			for _, sample := range src {
				dst[i] = PCMAtoPCM(sample)
				i++
			}
			return
		}
	}

	if src.Channels > 1 {
		filters = append(filters, Downsample(float32(src.Channels)))
	}

	if src.ClockRate > dst.ClockRate {
		filters = append(filters, Downsample(float32(src.ClockRate)/float32(dst.ClockRate)))
	} else if src.ClockRate < dst.ClockRate {
		filters = append(filters, Upsample(float32(dst.ClockRate)/float32(src.ClockRate)))
	}

	if dst.Channels > 1 {
		filters = append(filters, Upsample(float32(dst.Channels)))
	}

	switch dst.Name {
	case core.CodecPCML:
		writer = func(src []int16) (dst []byte) {
			var i int
			dst = make([]byte, len(src)*2)
			for _, sample := range src {
				dst[i] = byte(sample)
				i++
				dst[i] = byte(sample >> 8)
				i++
			}
			return
		}
	case core.CodecPCM:
		writer = func(src []int16) (dst []byte) {
			var i int
			dst = make([]byte, len(src)*2)
			for _, sample := range src {
				dst[i] = byte(sample >> 8)
				i++
				dst[i] = byte(sample)
				i++
			}
			return
		}
	case core.CodecPCMU:
		writer = func(src []int16) (dst []byte) {
			var i int
			dst = make([]byte, len(src))
			for _, sample := range src {
				dst[i] = PCMtoPCMU(sample)
				i++
			}
			return
		}
	case core.CodecPCMA:
		writer = func(src []int16) (dst []byte) {
			var i int
			dst = make([]byte, len(src))
			for _, sample := range src {
				dst[i] = PCMtoPCMA(sample)
				i++
			}
			return
		}
	}

	return func(b []byte) []byte {
		samples := reader(b)
		for _, filter := range filters {
			samples = filter(samples)
		}
		return writer(samples)
	}
}

func ConsumerCodecs() []*core.Codec {
	return []*core.Codec{
		{Name: core.CodecPCML},
		{Name: core.CodecPCM},
		{Name: core.CodecPCMA},
		{Name: core.CodecPCMU},
	}
}

func ProducerCodecs() []*core.Codec {
	return []*core.Codec{
		{Name: core.CodecPCML, ClockRate: 16000},
		{Name: core.CodecPCM, ClockRate: 16000},
		{Name: core.CodecPCML, ClockRate: 8000},
		{Name: core.CodecPCM, ClockRate: 8000},
		{Name: core.CodecPCMA, ClockRate: 8000},
		{Name: core.CodecPCMU, ClockRate: 8000},
		{Name: core.CodecPCML, ClockRate: 22050}, // wyoming-snd-external
	}
}
