// Package pcm - support raw (verbatim) PCM 16 bit in the FLAC container:
// - only 1 channel
// - only 16 bit per sample
// - only 8000, 16000, 24000, 48000 sample rate
package pcm

import (
	"encoding/binary"
	"unicode/utf8"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
	"github.com/sigurn/crc16"
	"github.com/sigurn/crc8"
)

func FLACHeader(magic bool, sampleRate uint32) []byte {
	b := make([]byte, 42)

	if magic {
		copy(b, "fLaC") // [0..3]
	}

	// https://xiph.org/flac/format.html#metadata_block_header
	b[4] = 0x80 // [4] lastMetadata=1 (1 bit), blockType=0 - STREAMINFO (7 bit)
	b[7] = 0x22 // [5..7] blockLength=34 (24 bit)

	// Important for Apple QuickTime player:
	// 1. Both values should be same
	// 2. Maximum value = 32768
	binary.BigEndian.PutUint16(b[8:], 32768)  // [8..9] info.BlockSizeMin=16 (16 bit)
	binary.BigEndian.PutUint16(b[10:], 32768) // [10..11] info.BlockSizeMin=65535 (16 bit)

	// [12..14] info.FrameSizeMin=0 (24 bit)
	// [15..17] info.FrameSizeMax=0 (24 bit)

	b[18] = byte(sampleRate >> 12)
	b[19] = byte(sampleRate >> 4)
	b[20] = byte(sampleRate << 4) // [18..20] info.SampleRate=8000 (20 bit), info.NChannels=1-1 (3 bit)

	b[21] = 0xF0 // [21..25] info.BitsPerSample=16-1 (5 bit), info.NSamples (36 bit)

	// [26..41] MD5sum (16 bytes)

	return b
}

var table8 *crc8.Table
var table16 *crc16.Table

func FLACEncoder(codecName string, clockRate uint32, handler core.HandlerFunc) core.HandlerFunc {
	var sr byte
	switch clockRate {
	case 8000:
		sr = 0b0100
	case 16000:
		sr = 0b0101
	case 22050:
		sr = 0b0110
	case 24000:
		sr = 0b0111
	case 32000:
		sr = 0b1000
	case 44100:
		sr = 0b1001
	case 48000:
		sr = 0b1010
	case 96000:
		sr = 0b1011
	default:
		return nil
	}

	if table8 == nil {
		table8 = crc8.MakeTable(crc8.CRC8)
	}
	if table16 == nil {
		table16 = crc16.MakeTable(crc16.CRC16_BUYPASS)
	}

	var sampleNumber int32

	return func(packet *rtp.Packet) {
		samples := uint16(len(packet.Payload))

		if codecName == core.CodecPCM || codecName == core.CodecPCML {
			samples /= 2
		}

		// https://xiph.org/flac/format.html#frame_header
		buf := make([]byte, samples*2+30)

		// 1. Frame header
		buf[0] = 0xFF
		buf[1] = 0xF9      // [0..1] syncCode=0xFFF8 - reserved (15 bit), blockStrategy=1 - variable-blocksize (1 bit)
		buf[2] = 0x70 | sr // blockSizeType=7 (4 bit), sampleRate=4 - 8000 (4 bit)
		buf[3] = 0x08      // channels=1-1 (4 bit), sampleSize=4 - 16 (3 bit), reserved=0 (1 bit)

		n := 4 + utf8.EncodeRune(buf[4:], sampleNumber) // 4 bytes max
		sampleNumber += int32(samples)

		// this is wrong but very simple frame block size value
		binary.BigEndian.PutUint16(buf[n:], samples-1)
		n += 2

		buf[n] = crc8.Checksum(buf[:n], table8)
		n += 1

		// 2. Subframe header
		buf[n] = 0x02 // padding=0 (1 bit), subframeType=1 - verbatim (6 bit), wastedFlag=0 (1 bit)
		n += 1

		// 3. Subframe
		switch codecName {
		case core.CodecPCMA:
			for _, b := range packet.Payload {
				s16 := PCMAtoPCM(b)
				buf[n] = byte(s16 >> 8)
				buf[n+1] = byte(s16)
				n += 2
			}
		case core.CodecPCMU:
			for _, b := range packet.Payload {
				s16 := PCMUtoPCM(b)
				buf[n] = byte(s16 >> 8)
				buf[n+1] = byte(s16)
				n += 2
			}
		case core.CodecPCM:
			n += copy(buf[n:], packet.Payload)
		case core.CodecPCML:
			// reverse endian from little to big
			size := len(packet.Payload)
			for i := 0; i < size; i += 2 {
				buf[n] = packet.Payload[i+1]
				buf[n+1] = packet.Payload[i]
				n += 2
			}
		}

		// 4. Frame footer
		crc := crc16.Checksum(buf[:n], table16)
		binary.BigEndian.PutUint16(buf[n:], crc)
		n += 2

		clone := *packet
		clone.Payload = buf[:n]

		handler(&clone)
	}
}
