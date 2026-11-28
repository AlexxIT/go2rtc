package aac

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/skrashevich/go-aac/pkg/decoder"
)

func newDecoder(codec *core.Codec) (*decoder.Decoder, error) {
	conf, err := hex.DecodeString(strings.TrimSpace(core.Between(codec.FmtpLine, "config=", ";")))
	if err != nil || len(conf) < 2 {
		return nil, fmt.Errorf("aac: invalid AudioSpecificConfig")
	}
	d := decoder.New()
	if err = d.SetASC(conf); err != nil {
		return nil, err
	}
	if d.Config.Profile != TypeAACLC && d.Config.Profile != TypeAACELD {
		return nil, fmt.Errorf("aac: built-in decoder requires AAC-LC or AAC-ELD")
	}
	// HomeKit ELD advertises a playback clock separately from its decoder
	// config (the source uses a 24 kHz ASC for FFmpeg compatibility).
	homekit := codec.Name == core.CodecELD && d.Config.Profile == TypeAACELD
	if d.Config.ChanConfig < 1 || d.Config.ChanConfig > 2 || d.Config.SampleRate <= 0 || d.Config.SampleIndex >= 13 ||
		sampleRates[d.Config.SampleIndex] != uint32(d.Config.SampleRate) ||
		!homekit && codec.ClockRate != 0 && codec.ClockRate != uint32(d.Config.SampleRate) ||
		codec.Channels != 0 && codec.Channels != uint8(d.Config.ChanConfig) {
		return nil, fmt.Errorf("aac: unsupported or inconsistent audio configuration")
	}
	return d, nil
}

// PCMCodec describes the interleaved S16LE output of the AAC decoder.
func PCMCodec(src *core.Codec) (*core.Codec, error) {
	d, err := newDecoder(src)
	if err != nil {
		return nil, err
	}
	rate := uint32(d.Config.SampleRate)
	if src.Name == core.CodecELD && src.ClockRate != 0 {
		rate = src.ClockRate
	}
	return &core.Codec{Name: core.CodecPCML, ClockRate: rate,
		Channels: uint8(d.Config.ChanConfig), PayloadType: core.PayloadTypeRAW}, nil
}

// DecodeHandler accepts raw/ADTS access units and complete MPEG4-GENERIC
// AAC-hbr RTP packets with 13-bit size and 3-bit index headers.
func DecodeHandler(src *core.Codec, handler core.HandlerFunc) (core.HandlerFunc, error) {
	d, err := newDecoder(src)
	if err != nil {
		return nil, err
	}
	if src.IsRTP() {
		if mode := strings.TrimSpace(core.Between(src.FmtpLine, "mode=", ";")); mode != "" && !strings.EqualFold(mode, "AAC-hbr") {
			return nil, fmt.Errorf("aac: unsupported RTP mode %s", mode)
		}
		for key, value := range map[string]string{"sizelength": "13", "indexlength": "3", "indexdeltalength": "3"} {
			if v := strings.TrimSpace(core.Between(src.FmtpLine, key+"=", ";")); v != "" && v != value {
				return nil, fmt.Errorf("aac: unsupported RTP %s=%s", key, v)
			}
		}
	}
	var sequence uint16
	auTime := uint32(d.Config.FrameLength)
	var mu sync.Mutex
	decode := func(packet *core.Packet, payload []byte, timestamp uint32) {
		if IsADTS(payload) {
			codec := ADTSToCodec(payload)
			if codec.ClockRate != uint32(d.Config.SampleRate) || codec.Channels != uint8(d.Config.ChanConfig) || payload[2]>>6 != TypeAACLC-1 || payload[6]&3 != 0 {
				return
			}
			size, header := int(ReadADTSSize(payload)), ADTSHeaderLen(payload)
			if size != len(payload) || size < header {
				return
			}
			payload = payload[header:]
		}
		samples, err := d.DecodeFrame(payload)
		if err != nil || len(samples) != int(auTime)*int(d.Config.ChanConfig) {
			return
		}
		out := *packet
		out.Payload = make([]byte, len(samples)*2)
		for i, v := range samples {
			if math.IsNaN(float64(v)) {
				v = 0
			}
			sample := int16(max(-32768, min(32767, float64(v)*32767)))
			binary.LittleEndian.PutUint16(out.Payload[i*2:], uint16(sample))
		}
		out.Version, out.PayloadType, out.SequenceNumber, out.Timestamp = 2, core.PayloadTypeRAW, sequence, timestamp
		sequence++
		handler(&out)
	}
	return func(packet *core.Packet) {
		mu.Lock()
		defer mu.Unlock()
		payload := packet.Payload
		if len(payload) == 0 {
			return
		}
		if src.IsRTP() && packet.Version == 2 && !IsADTS(payload) {
			if len(payload) < 4 {
				return
			}
			bits := int(binary.BigEndian.Uint16(payload))
			if bits == 0 || bits%16 != 0 || len(payload) < 2+bits/8 {
				return
			}
			headers, units := payload[2:2+bits/8], payload[2+bits/8:]
			// Validate the entire packet before advancing decoder state.
			size := 0
			for i := 0; i < len(headers); i += 2 {
				h := binary.BigEndian.Uint16(headers[i:])
				if h&7 != 0 {
					return
				}
				size += int(h >> 3)
			}
			if size != len(units) {
				return
			} // fragmented/truncated AUs aren't complete frames
			for i := 0; i < len(headers); i += 2 {
				n := int(binary.BigEndian.Uint16(headers[i:]) >> 3)
				decode(packet, units[:n], packet.Timestamp+uint32(i/2)*auTime)
				units = units[n:]
			}
		} else if IsADTS(payload) {
			timestamp := packet.Timestamp
			for IsADTS(payload) {
				n := int(ReadADTSSize(payload))
				if n < ADTSHeaderLen(payload) || n > len(payload) {
					return
				}
				decode(packet, payload[:n], timestamp)
				payload, timestamp = payload[n:], timestamp+auTime
			}
		} else {
			decode(packet, payload, packet.Timestamp)
		}
	}, nil
}
