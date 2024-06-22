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

	for _, codec := range m.codecs {
		switch codec.Name {
		case core.CodecH264:
			b[4] |= FlagsVideo
			obj["videocodecid"] = CodecAVC

		case core.CodecAAC:
			b[4] |= FlagsAudio
			obj["audiocodecid"] = CodecAAC
			obj["audiosamplerate"] = codec.ClockRate
			obj["audiosamplesize"] = 16
			obj["stereo"] = codec.Channels == 2
		}
	}

	data := amf.EncodeItems("@setDataFrame", "onMetaData", obj)
	b = append(b, EncodeTag(TagData, 0, data)...)

	for _, codec := range m.codecs {
		switch codec.Name {
		case core.CodecH264:
			sps, pps := h264.GetParameterSet(codec.FmtpLine)
			if len(sps) == 0 {
				sps = []byte{0x67, 0x42, 0x00, 0x0a, 0xf8, 0x41, 0xa2}
			} else {
				h264.FixPixFmt(sps)
			}
			if len(pps) == 0 {
				pps = []byte{0x68, 0xce, 0x38, 0x80}
			}

			config := h264.EncodeConfig(sps, pps)
			video := append(encodeAVData(codec, 0), config...)
			b = append(b, EncodeTag(TagVideo, 0, video)...)

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

		return func(packet *rtp.Packet) []byte {
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
			return EncodeTag(TagVideo, timeMS, buf)
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
