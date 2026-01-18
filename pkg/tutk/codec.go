package tutk

// https://github.com/seydx/tutk_wyze#11-codec-reference
const (
	CodecMPEG4 byte = 0x4C
	CodecH263  byte = 0x4D
	CodecH264  byte = 0x4E
	CodecMJPEG byte = 0x4F
	CodecH265  byte = 0x50
)

const (
	CodecAACRaw  byte = 0x86
	CodecAACADTS byte = 0x87
	CodecAACLATM byte = 0x88
	CodecPCMU    byte = 0x89
	CodecPCMA    byte = 0x8A
	CodecADPCM   byte = 0x8B
	CodecPCML    byte = 0x8C
	CodecSPEEX   byte = 0x8D
	CodecMP3     byte = 0x8E
	CodecG726    byte = 0x8F
	CodecAACAlt  byte = 0x90
	CodecOpus    byte = 0x92
)

var sampleRates = [9]uint32{8000, 11025, 12000, 16000, 22050, 24000, 32000, 44100, 48000}

func GetSampleRateIndex(sampleRate uint32) uint8 {
	for i, rate := range sampleRates {
		if rate == sampleRate {
			return uint8(i)
		}
	}
	return 3 // default 16kHz
}

func GetSamplesPerFrame(codecID byte) uint32 {
	switch codecID {
	case CodecAACRaw, CodecAACADTS, CodecAACLATM, CodecAACAlt:
		return 1024
	case CodecPCMU, CodecPCMA, CodecPCML, CodecADPCM, CodecSPEEX, CodecG726:
		return 160
	case CodecMP3:
		return 1152
	case CodecOpus:
		return 960
	default:
		return 1024
	}
}

func IsVideoCodec(id byte) bool {
	return id >= CodecMPEG4 && id <= CodecH265
}

func IsAudioCodec(id byte) bool {
	return id >= CodecAACRaw && id <= CodecOpus
}
