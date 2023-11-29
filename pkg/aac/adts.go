package aac

import (
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/bits"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func IsADTS(b []byte) bool {
	_ = b[1]
	return len(b) > 7 && b[0] == 0xFF && b[1]&0xF6 == 0xF0
}

func ADTSToCodec(b []byte) *core.Codec {
	// 1. Check ADTS header
	if !IsADTS(b) {
		return nil
	}

	// 2. Decode ADTS params
	// https://wiki.multimedia.cx/index.php/ADTS
	rd := bits.NewReader(b)
	_ = rd.ReadBits(12)              // Syncword, all bits must be set to 1
	_ = rd.ReadBit()                 // MPEG Version, set to 0 for MPEG-4 and 1 for MPEG-2
	_ = rd.ReadBits(2)               // Layer, always set to 0
	_ = rd.ReadBit()                 // Protection absence, set to 1 if there is no CRC and 0 if there is CRC
	objType := rd.ReadBits8(2) + 1   // Profile, the MPEG-4 Audio Object Type minus 1
	sampleRateIdx := rd.ReadBits8(4) // MPEG-4 Sampling Frequency Index
	_ = rd.ReadBit()                 // Private bit, guaranteed never to be used by MPEG, set to 0 when encoding, ignore when decoding
	channels := rd.ReadBits16(3)     // MPEG-4 Channel Configuration

	//_ = rd.ReadBit()    // Originality, set to 1 to signal originality of the audio and 0 otherwise
	//_ = rd.ReadBit()    // Home, set to 1 to signal home usage of the audio and 0 otherwise
	//_ = rd.ReadBit()    // Copyright ID bit
	//_ = rd.ReadBit()    // Copyright ID start
	//_ = rd.ReadBits(13) // Frame length
	//_ = rd.ReadBits(11) // Buffer fullness
	//_ = rd.ReadBits(2)  // Number of AAC frames (Raw Data Blocks) in ADTS frame minus 1
	//_ = rd.ReadBits(16) // CRC check

	// 3. Encode RTP config
	wr := bits.NewWriter(nil)
	wr.WriteBits8(objType, 5)
	wr.WriteBits8(sampleRateIdx, 4)
	wr.WriteBits16(channels, 4)
	conf := wr.Bytes()

	codec := &core.Codec{
		Name:      core.CodecAAC,
		ClockRate: sampleRates[sampleRateIdx],
		Channels:  channels,
		FmtpLine:  FMTP + hex.EncodeToString(conf),
	}
	return codec
}

func ReadADTSSize(b []byte) uint16 {
	// AAAAAAAA AAAABCCD EEFFFFGH HHIJKLMM MMMMMMMM MMMOOOOO OOOOOOPP (QQQQQQQQ QQQQQQQQ)
	_ = b[5] // bounds
	return uint16(b[3]&0x03)<<(8+3) | uint16(b[4])<<3 | uint16(b[5]>>5)
}

func WriteADTSSize(b []byte, size uint16) {
	// AAAAAAAA AAAABCCD EEFFFFGH HHIJKLMM MMMMMMMM MMMOOOOO OOOOOOPP (QQQQQQQQ QQQQQQQQ)
	_ = b[5] // bounds
	b[3] |= byte(size >> (8 + 3))
	b[4] = byte(size >> 3)
	b[5] |= byte(size << 5)
	return
}

func ADTSTimeSize(b []byte) uint32 {
	var units uint32
	for len(b) > ADTSHeaderSize {
		auSize := ReadADTSSize(b)
		b = b[auSize:]
		units++
	}
	return units * AUTime
}

func CodecToADTS(codec *core.Codec) []byte {
	s := core.Between(codec.FmtpLine, "config=", ";")
	conf, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}

	objType, sampleFreqIdx, channels, _ := DecodeConfig(conf)
	profile := objType - 1

	wr := bits.NewWriter(nil)
	wr.WriteAllBits(1, 12)          // Syncword, all bits must be set to 1
	wr.WriteBit(0)                  // MPEG Version, set to 0 for MPEG-4 and 1 for MPEG-2
	wr.WriteBits8(0, 2)             // Layer, always set to 0
	wr.WriteBit(1)                  // Protection absence, set to 1 if there is no CRC and 0 if there is CRC
	wr.WriteBits8(profile, 2)       // Profile, the MPEG-4 Audio Object Type minus 1
	wr.WriteBits8(sampleFreqIdx, 4) // MPEG-4 Sampling Frequency Index
	wr.WriteBit(0)                  // Private bit, guaranteed never to be used by MPEG, set to 0 when encoding, ignore when decoding
	wr.WriteBits8(channels, 3)      // MPEG-4 Channel Configuration
	wr.WriteBit(0)                  // Originality, set to 1 to signal originality of the audio and 0 otherwise
	wr.WriteBit(0)                  // Home, set to 1 to signal home usage of the audio and 0 otherwise
	wr.WriteBit(0)                  // Copyright ID bit
	wr.WriteBit(0)                  // Copyright ID start
	wr.WriteBits16(0, 13)           // Frame length
	wr.WriteAllBits(1, 11)          // Buffer fullness (variable bitrate)
	wr.WriteBits8(0, 2)             // Number of AAC frames (Raw Data Blocks) in ADTS frame minus 1

	return wr.Bytes()
}

func EncodeToADTS(codec *core.Codec, handler core.HandlerFunc) core.HandlerFunc {
	adts := CodecToADTS(codec)

	return func(packet *rtp.Packet) {
		if !IsADTS(packet.Payload) {
			b := make([]byte, ADTSHeaderSize+len(packet.Payload))
			copy(b, adts)
			copy(b[ADTSHeaderSize:], packet.Payload)
			WriteADTSSize(b, uint16(len(b)))

			clone := *packet
			clone.Payload = b
			handler(&clone)
		} else {
			handler(packet)
		}
	}
}
