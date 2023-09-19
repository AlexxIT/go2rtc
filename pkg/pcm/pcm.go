package pcm

import (
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// ResampleToG711 - convert PCMA/PCM/PCML to PCMA and PCMU to PCMU with decreasing sample rate
func ResampleToG711(codec *core.Codec, sampleRate uint32, handler core.HandlerFunc) core.HandlerFunc {
	n := float32(codec.ClockRate) / float32(sampleRate)

	if codec.Channels == 2 {
		n *= 2 // hacky way for support two channels audio
	}

	switch codec.Name {
	case core.CodecPCMA:
		return DownsampleByte(PCMAtoPCM, PCMtoPCMA, n, handler)
	case core.CodecPCMU:
		return DownsampleByte(PCMUtoPCM, PCMtoPCMU, n, handler)
	case core.CodecPCM, core.CodecPCML:
		if n == 1 {
			handler = ResamplePCM(PCMtoPCMA, handler)
		} else {
			handler = DownsamplePCM(PCMtoPCMA, n, handler)
		}

		if codec.Name == core.CodecPCML {
			return LittleToBig(handler)
		}

		return handler
	}

	panic(core.Caller())
}

// DownsampleByte - convert PCMA/PCMU to PCMA/PCMU with decreasing sample rate (N times)
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

// LittleToBig - conver PCM little endian to PCM big endian
func LittleToBig(handler core.HandlerFunc) core.HandlerFunc {
	return func(packet *rtp.Packet) {
		size := len(packet.Payload)
		b := make([]byte, size)
		for i := 0; i < size; i += 2 {
			b[i] = packet.Payload[i+1]
			b[i+1] = packet.Payload[i]
		}

		clone := *packet
		clone.Payload = b
		handler(&clone)
	}
}

// ResamplePCM - convert PCM to PCMA/PCMU with same sample rate
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

// DownsamplePCM - convert PCM to PCMA/PCMU with decreasing sample rate (N times)
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

// RepackG711 - Repack G.711 PCMA/PCMU into frames of size 1024
//  1. Fixes WebRTC audio quality issue (monotonic timestamp)
//  2. Fixes Reolink Doorbell backchannel issue (zero timestamp)
//     https://github.com/AlexxIT/go2rtc/issues/331
func RepackG711(zeroTS bool, handler core.HandlerFunc) core.HandlerFunc {
	const PacketSize = 1024

	var buf []byte
	var seq uint16
	var ts uint32

	// fix https://github.com/AlexxIT/go2rtc/issues/432
	var mu sync.Mutex

	return func(packet *rtp.Packet) {
		mu.Lock()

		buf = append(buf, packet.Payload...)
		if len(buf) < PacketSize {
			mu.Unlock()
			return
		}

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         true,               // should be true
				PayloadType:    packet.PayloadType, // will be owerwriten
				SequenceNumber: seq,
				SSRC:           packet.SSRC,
			},
			Payload: buf[:PacketSize],
		}

		seq++

		// don't know if zero TS important for Reolink Doorbell
		// don't have this strange devices for tests
		if !zeroTS {
			pkt.Timestamp = ts
			ts += PacketSize
		}

		buf = buf[PacketSize:]

		mu.Unlock()

		handler(pkt)
	}
}
