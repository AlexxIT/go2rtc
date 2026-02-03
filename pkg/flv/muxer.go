package flv

import (
	"encoding/binary"
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/pion/rtp"
)

type Muxer struct {
	codecs []*core.Codec
}

const (
	FlagsVideo = 0b001
	FlagsAudio = 0b100
)

func (m *Muxer) GetInit() []byte {
	b := []byte{
		'F', 'L', 'V', // signature
		1,          // version
		0,          // flags (has video/audio)
		0, 0, 0, 9, // header size
		0, 0, 0, 0, // tag 0 size
	}

	obj := map[string]any{}

	var metaWidth, metaHeight uint16
	var metaFPS float64

	for _, codec := range m.codecs {
		switch codec.Name {
		case core.CodecH264:
			b[4] |= FlagsVideo
			obj["videocodecid"] = CodecH264

			// Try to extract width/height and optional FPS from SPS
			if sps, _ := h264.GetParameterSet(codec.FmtpLine); len(sps) > 0 {
				if s := h264.DecodeSPS(sps); s != nil {
					if metaWidth == 0 || metaHeight == 0 {
						metaWidth = s.Width()
						metaHeight = s.Height()
					}
					if f := s.FPS(); f > 0 {
						metaFPS = f
					}
				}
			}

		case core.CodecAAC:
			b[4] |= FlagsAudio
			obj["audiocodecid"] = CodecAAC
			obj["audiosamplerate"] = codec.ClockRate
			obj["audiosamplesize"] = 16
			obj["stereo"] = codec.Channels == 2
		}
	}

	// Fill optional width/height/framerate if known
	if metaWidth > 0 && metaHeight > 0 {
		obj["width"] = metaWidth
		obj["height"] = metaHeight
	}
	if metaFPS > 0 {
		obj["framerate"] = metaFPS
	}

	data := amf.EncodeItems("@setDataFrame", "onMetaData", obj)
	b = append(b, EncodeTag(TagData, 0, data)...)

	for _, codec := range m.codecs {
		switch codec.Name {
		case core.CodecH264:
			sps, pps := h264.GetParameterSet(codec.FmtpLine)
			if len(sps) > 0 && len(pps) > 0 {
				h264.FixPixFmt(sps)
				config := h264.EncodeConfig(sps, pps)
				video := append(encodeAVData(codec, 0), config...)
				b = append(b, EncodeTag(TagVideo, 0, video)...)
			}

		case core.CodecAAC:
			s := core.Between(codec.FmtpLine, "config=", ";")
			config, _ := hex.DecodeString(s)
			audio := append(encodeAVData(codec, 0), config...)
			b = append(b, EncodeTag(TagAudio, 0, audio)...)
		}
	}

	return b
}

func (m *Muxer) GetPayloader(codec *core.Codec) func(packet *rtp.Packet) []byte {
	m.codecs = append(m.codecs, codec)

	var ts0 uint32
	var k = codec.ClockRate / 1000

	switch codec.Name {
	case core.CodecH264:
		buf := encodeAVData(codec, 1)
		// Some RTSP servers (FFmpeg) don't provide sprop-parameter-sets in SDP.
		// That makes initial FLV sequence header fallback to a generic SPS/PPS,
		// which can confuse some RTMP ingests. Emit a real AVC sequence header
		// once we see SPS/PPS inside the first Access Unit.
		var sentRealHeader bool

		return func(packet *rtp.Packet) []byte {
			var header []byte
			if !sentRealHeader {
				// Try to extract SPS/PPS from the current AVCC payload
				var sps, pps []byte
				for _, nalu := range h264.SplitNALU(packet.Payload) {
					switch h264.NALUType(nalu) {
					case h264.NALUTypeSPS:
						sps = nalu[4:]
					case h264.NALUTypePPS:
						pps = nalu[4:]
					}
				}
				if len(sps) > 0 && len(pps) > 0 {
					conf := h264.EncodeConfig(sps, pps)
					hdr := append(encodeAVData(codec, 0), conf...)
					// Propagate discovered SPS/PPS into codec fmtp so late joiners (e.g., WebRTC)
					// have sprop-parameter-sets available.
					if c := h264.ConfigToCodec(conf); c != nil {
						codec.FmtpLine = c.FmtpLine
					}
					if ts0 == 0 {
						ts0 = packet.Timestamp
					}
					timeMS := (packet.Timestamp - ts0) / k
					header = EncodeTag(TagVideo, timeMS, hdr)
					sentRealHeader = true
				}
			}

			if h264.IsKeyframe(packet.Payload) {
				buf[0] = 1<<4 | 7
			} else {
				buf[0] = 2<<4 | 7
			}

			buf = append(buf[:5], packet.Payload...) // reset buffer to previous place

			if ts0 == 0 {
				ts0 = packet.Timestamp
			}

			timeMS := (packet.Timestamp - ts0) / k
			frame := EncodeTag(TagVideo, timeMS, buf)
			if len(header) > 0 {
				// Emit real config immediately before the first frame containing SPS/PPS
				out := make([]byte, 0, len(header)+len(frame))
				out = append(out, header...)
				out = append(out, frame...)
				return out
			}
			return frame
		}

	case core.CodecAAC:
		buf := encodeAVData(codec, 1)

		return func(packet *rtp.Packet) []byte {
			buf = append(buf[:2], packet.Payload...)

			if ts0 == 0 {
				ts0 = packet.Timestamp
			}

			timeMS := (packet.Timestamp - ts0) / k
			return EncodeTag(TagAudio, timeMS, buf)
		}
	}

	return nil
}

func EncodeTag(tagType byte, timeMS uint32, payload []byte) []byte {
	payloadSize := uint32(len(payload))
	tagSize := payloadSize + 11

	b := make([]byte, tagSize+4)
	b[0] = tagType
	b[1] = byte(payloadSize >> 16)
	b[2] = byte(payloadSize >> 8)
	b[3] = byte(payloadSize)
	b[4] = byte(timeMS >> 16)
	b[5] = byte(timeMS >> 8)
	b[6] = byte(timeMS)
	b[7] = byte(timeMS >> 24)
	copy(b[11:], payload)

	binary.BigEndian.PutUint32(b[tagSize:], tagSize)
	return b
}

func encodeAVData(codec *core.Codec, isFrame byte) []byte {
	switch codec.Name {
	case core.CodecH264:
		return []byte{
			1<<4 | 7, // keyframe + AVC
			isFrame,  // 0 - config, 1 - frame
			0, 0, 0,  // composition time = 0
		}

	case core.CodecAAC:
		var b0 byte = 10 << 4 // AAC

		switch codec.ClockRate {
		case 11025:
			b0 |= 1 << 2
		case 22050:
			b0 |= 2 << 2
		case 44100:
			b0 |= 3 << 2
		}

		b0 |= 1 << 1 // 16 bits

		if codec.Channels == 2 {
			b0 |= 1
		}

		return []byte{b0, isFrame} // 0 - config, 1 - frame
	}

	return nil
}
