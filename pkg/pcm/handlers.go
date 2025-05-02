package pcm

import (
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

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

// LittleToBig - convert PCM little endian to PCM big endian
func LittleToBig(handler core.HandlerFunc) core.HandlerFunc {
	return func(packet *rtp.Packet) {
		clone := *packet
		clone.Payload = FlipEndian(packet.Payload)
		handler(&clone)
	}
}

func TranscodeHandler(dst, src *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	var ts uint32
	k := float32(BytesPerFrame(dst)) / float32(BytesPerFrame(src))
	f := Transcode(dst, src)

	return func(packet *rtp.Packet) {
		ts += uint32(k * float32(len(packet.Payload)))

		clone := *packet
		clone.Payload = f(packet.Payload)
		clone.Timestamp = ts
		handler(&clone)
	}
}

func BytesPerSample(codec *core.Codec) int {
	switch codec.Name {
	case core.CodecPCML, core.CodecPCM:
		return 2
	case core.CodecPCMU, core.CodecPCMA:
		return 1
	}
	return 0
}

func BytesPerFrame(codec *core.Codec) int {
	if codec.Channels <= 1 {
		return BytesPerSample(codec)
	}
	return int(codec.Channels) * BytesPerSample(codec)
}

func FramesPerDuration(codec *core.Codec, duration time.Duration) int {
	return int(time.Duration(codec.ClockRate) * duration / time.Second)
}

func BytesPerDuration(codec *core.Codec, duration time.Duration) int {
	return BytesPerFrame(codec) * FramesPerDuration(codec, duration)
}
