package opus

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/pcm"
	goopus "github.com/skrashevich/go-opus"
)

func CanTranscode(src, dst *core.Codec) bool {
	if src == nil || dst == nil || src.Channels > 2 || dst.Channels > 2 {
		return false
	}
	return src.Name == core.CodecOpus && isLinearOrG711(dst.Name) || dst.Name == core.CodecOpus && isLinearOrG711(src.Name) && src.ClockRate != 0
}

func isLinearOrG711(name string) bool {
	return isPCM(name) || name == core.CodecPCMA || name == core.CodecPCMU
}

func outputCodec(dst, src *core.Codec) *core.Codec {
	dst = dst.Clone()
	if dst.Name == core.CodecOpus {
		dst.ClockRate = 48000
	}
	if dst.ClockRate == 0 {
		dst.ClockRate = 48000
		if dst.Name == core.CodecPCMA || dst.Name == core.CodecPCMU {
			dst.ClockRate = 8000
		}
	}
	if dst.Channels == 0 {
		dst.Channels = max(src.Channels, 1)
		if dst.Name == core.CodecPCMA || dst.Name == core.CodecPCMU {
			dst.Channels = 1
		}
	}
	return dst
}

// TranscodeHandler converts PCM/G.711 ↔ Opus. Each handler owns an independent
// Go codec instance. go-opus reclaims its native resources automatically.
func TranscodeHandler(dst, src *core.Codec, handler core.HandlerFunc) (core.HandlerFunc, error) {
	if !CanTranscode(src, dst) {
		return nil, fmt.Errorf("opus: unsupported conversion %v => %v", src, dst)
	}
	dst = outputCodec(dst, src)
	if src.Name == core.CodecOpus {
		return decodeHandler(dst, src, handler)
	}
	return encodeHandler(dst, src, handler)
}

func encodeHandler(dst, src *core.Codec, handler core.HandlerFunc) (core.HandlerFunc, error) {
	encoder, err := goopus.NewEncoder(goopus.WithChannels(int(dst.Channels)), goopus.WithVBR(true))
	if err != nil {
		return nil, err
	}
	convert := pcm.TranscodeChannels(&core.Codec{Name: core.CodecPCML, ClockRate: 48000, Channels: dst.Channels}, src)
	frameBytes := 960 * int(dst.Channels) * 2
	var pending []byte
	var sequence uint16
	var timestamp, expected uint32
	var started bool
	var mu sync.Mutex
	return func(packet *core.Packet) {
		mu.Lock()
		defer mu.Unlock()
		if len(packet.Payload)%pcm.BytesPerFrame(src) != 0 {
			return
		}
		if !started || packet.Timestamp != 0 && packet.Timestamp != expected {
			pending = nil
			timestamp = uint32(uint64(packet.Timestamp) * 48000 / uint64(src.ClockRate))
			started = true
		}
		expected = packet.Timestamp + uint32(len(packet.Payload)/pcm.BytesPerFrame(src))
		pending = append(pending, convert(packet.Payload)...)
		for len(pending) >= frameBytes {
			data := make([]byte, 1276)
			n, err := encoder.Encode(pending[:frameBytes], data)
			if err != nil {
				pending = nil
				return
			}
			out := *packet
			out.Payload, out.SequenceNumber, out.Timestamp, out.PayloadType = data[:n], sequence, timestamp, dst.PayloadType
			handler(&out)
			sequence++
			timestamp += 960
			pending = pending[frameBytes:]
		}
	}, nil
}

func decodeHandler(dst, src *core.Codec, handler core.HandlerFunc) (core.HandlerFunc, error) {
	// Decode at 48 kHz and use the existing PCM converter for other outputs.
	decoder, err := goopus.NewDecoderWithOutput(48000, int(dst.Channels))
	if err != nil {
		return nil, err
	}
	convert := pcm.TranscodeChannels(dst, &core.Codec{Name: core.CodecPCML, ClockRate: 48000, Channels: dst.Channels})
	sourceRate := src.ClockRate
	if sourceRate == 0 {
		sourceRate = 48000
	}
	samples := make([]int16, 5760*int(dst.Channels))
	var sequence, previousSeq uint16
	var timestamp, previousTS uint32
	var started bool
	var mu sync.Mutex
	emit := func(packet *core.Packet, samples []int16) {
		raw := make([]byte, len(samples)*2)
		for i, sample := range samples {
			binary.LittleEndian.PutUint16(raw[i*2:], uint16(sample))
		}
		out := *packet
		out.Payload = convert(raw)
		out.SequenceNumber, out.Timestamp, out.PayloadType = sequence, timestamp, dst.PayloadType
		handler(&out)
		sequence++
		timestamp += uint32(len(out.Payload) / pcm.BytesPerFrame(dst))
	}
	return func(packet *core.Packet) {
		mu.Lock()
		defer mu.Unlock()
		if started {
			advance := uint16(packet.SequenceNumber - previousSeq)
			if advance == 0 || advance > 0x8000 {
				return
			}
			gap := advance - 1
			if gap > 3 {
				timestamp = uint32(uint64(packet.Timestamp) * uint64(dst.ClockRate) / uint64(sourceRate))
			} else if gap > 0 {
				missing := int(uint64(uint32(packet.Timestamp-previousTS))*48000/uint64(sourceRate)/uint64(gap+1)) * int(dst.Channels)
				if missing > 0 && missing <= len(samples) {
					for range gap {
						if decoder.DecodePLC(samples[:missing]) == nil {
							emit(packet, samples[:missing])
						}
					}
				}
				// PLC may reject durations unsupported by the preceding mode.
				// The real packet must still retain its source timeline.
				timestamp = uint32(uint64(packet.Timestamp) * uint64(dst.ClockRate) / uint64(sourceRate))
			}
		} else {
			timestamp = uint32(uint64(packet.Timestamp) * uint64(dst.ClockRate) / uint64(sourceRate))
		}
		n, err := decoder.DecodeToInt16(packet.Payload, samples)
		if err != nil {
			return
		}
		started, previousSeq, previousTS = true, packet.SequenceNumber, packet.Timestamp
		emit(packet, samples[:n*int(dst.Channels)])
	}, nil
}

func isPCM(name string) bool { return name == core.CodecPCM || name == core.CodecPCML }
