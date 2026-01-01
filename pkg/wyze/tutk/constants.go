package tutk

const (
	CodecUnknown uint16 = 0x00 // Unknown codec
	CodecMPEG4   uint16 = 0x4C // 76 - MPEG4
	CodecH263    uint16 = 0x4D // 77 - H.263
	CodecH264    uint16 = 0x4E // 78 - H.264/AVC (common for Wyze)
	CodecMJPEG   uint16 = 0x4F // 79 - MJPEG
	CodecH265    uint16 = 0x50 // 80 - H.265/HEVC (common for Wyze)
)

const (
	AudioCodecAACRaw  uint16 = 0x86 // 134 - AAC raw format
	AudioCodecAACADTS uint16 = 0x87 // 135 - AAC with ADTS header
	AudioCodecAACLATM uint16 = 0x88 // 136 - AAC with LATM format
	AudioCodecG711U   uint16 = 0x89 // 137 - G.711 μ-law (PCMU)
	AudioCodecG711A   uint16 = 0x8A // 138 - G.711 A-law (PCMA)
	AudioCodecADPCM   uint16 = 0x8B // 139 - ADPCM
	AudioCodecPCM     uint16 = 0x8C // 140 - PCM 16-bit signed LE
	AudioCodecSPEEX   uint16 = 0x8D // 141 - Speex
	AudioCodecMP3     uint16 = 0x8E // 142 - MP3
	AudioCodecG726    uint16 = 0x8F // 143 - G.726
	// Wyze extensions (not in official SDK)
	AudioCodecAACWyze uint16 = 0x90 // 144 - Wyze AAC
	AudioCodecOpus    uint16 = 0x92 // 146 - Opus codec
)

const (
	SampleRate8K  uint8 = 0x00 // 8000 Hz
	SampleRate11K uint8 = 0x01 // 11025 Hz
	SampleRate12K uint8 = 0x02 // 12000 Hz
	SampleRate16K uint8 = 0x03 // 16000 Hz
	SampleRate22K uint8 = 0x04 // 22050 Hz
	SampleRate24K uint8 = 0x05 // 24000 Hz
	SampleRate32K uint8 = 0x06 // 32000 Hz
	SampleRate44K uint8 = 0x07 // 44100 Hz
	SampleRate48K uint8 = 0x08 // 48000 Hz
)

var SampleRates = map[uint8]int{
	SampleRate8K:  8000,
	SampleRate11K: 11025,
	SampleRate12K: 12000,
	SampleRate16K: 16000,
	SampleRate22K: 22050,
	SampleRate24K: 24000,
	SampleRate32K: 32000,
	SampleRate44K: 44100,
	SampleRate48K: 48000,
}

var SamplesPerFrame = map[uint16]uint32{
	AudioCodecAACRaw:  1024, // AAC frame = 1024 samples
	AudioCodecAACADTS: 1024,
	AudioCodecAACLATM: 1024,
	AudioCodecAACWyze: 1024,
	AudioCodecG711U:   160, // G.711 typically 20ms = 160 samples at 8kHz
	AudioCodecG711A:   160,
	AudioCodecPCM:     160,
	AudioCodecADPCM:   160,
	AudioCodecSPEEX:   160,
	AudioCodecMP3:     1152, // MP3 frame = 1152 samples
	AudioCodecG726:    160,
	AudioCodecOpus:    960, // Opus typically 20ms = 960 samples at 48kHz
}

const (
	IOTypeVideoStart           = 0x01FF
	IOTypeVideoStop            = 0x02FF
	IOTypeAudioStart           = 0x0300
	IOTypeAudioStop            = 0x0301
	IOTypeSpeakerStart         = 0x0350
	IOTypeSpeakerStop          = 0x0351
	IOTypeGetAudioOutFormatReq = 0x032A
	IOTypeGetAudioOutFormatRes = 0x032B
	IOTypeSetStreamCtrlReq     = 0x0320
	IOTypeSetStreamCtrlRes     = 0x0321
	IOTypeGetStreamCtrlReq     = 0x0322
	IOTypeGetStreamCtrlRes     = 0x0323
	IOTypeDevInfoReq           = 0x0340
	IOTypeDevInfoRes           = 0x0341
	IOTypeGetSupportStreamReq  = 0x0344
	IOTypeGetSupportStreamRes  = 0x0345
	IOTypeSetRecordReq         = 0x0310
	IOTypeSetRecordRes         = 0x0311
	IOTypeGetRecordReq         = 0x0312
	IOTypeGetRecordRes         = 0x0313
	IOTypePTZCommand           = 0x1001
	IOTypeReceiveFirstFrame    = 0x1002
	IOTypeGetEnvironmentReq    = 0x030A
	IOTypeGetEnvironmentRes    = 0x030B
	IOTypeSetVideoModeReq      = 0x030C
	IOTypeSetVideoModeRes      = 0x030D
	IOTypeGetVideoModeReq      = 0x030E
	IOTypeGetVideoModeRes      = 0x030F
	IOTypeSetTimeReq           = 0x0316
	IOTypeSetTimeRes           = 0x0317
	IOTypeGetTimeReq           = 0x0318
	IOTypeGetTimeRes           = 0x0319
	IOTypeSetWifiReq           = 0x0102
	IOTypeSetWifiRes           = 0x0103
	IOTypeGetWifiReq           = 0x0104
	IOTypeGetWifiRes           = 0x0105
	IOTypeListWifiAPReq        = 0x0106
	IOTypeListWifiAPRes        = 0x0107
	IOTypeSetMotionDetectReq   = 0x0306
	IOTypeSetMotionDetectRes   = 0x0307
	IOTypeGetMotionDetectReq   = 0x0308
	IOTypeGetMotionDetectRes   = 0x0309
)

const (
	CmdDiscoReq     uint16 = 0x0601
	CmdDiscoRes     uint16 = 0x0602
	CmdSessionReq   uint16 = 0x0402
	CmdSessionRes   uint16 = 0x0404
	CmdDataTX       uint16 = 0x0407
	CmdDataRX       uint16 = 0x0408
	CmdKeepaliveReq uint16 = 0x0427
	CmdKeepaliveRes uint16 = 0x0428
)

const (
	MagicAVLoginResp uint16 = 0x2100
	MagicIOCtrl      uint16 = 0x7000
	MagicChannelMsg  uint16 = 0x1000
	MagicACK         uint16 = 0x0009
	MagicAVLogin1    uint16 = 0x0000
	MagicAVLogin2    uint16 = 0x2000
)

const (
	ProtocolVersion uint16 = 0x000c // Version 12
)

const (
	DefaultCapabilities uint32 = 0x001f07fb
)

const (
	KCmdAuth               = 10000
	KCmdChallenge          = 10001
	KCmdChallengeResp      = 10002
	KCmdAuthResult         = 10003
	KCmdControlChannel     = 10010
	KCmdControlChannelResp = 10011
	KCmdSetResolution      = 10056
	KCmdSetResolutionResp  = 10057
)

const (
	MediaTypeVideo       = 1
	MediaTypeAudio       = 2
	MediaTypeReturnAudio = 3
	MediaTypeRDT         = 4
)

const (
	IOTCChannelMain = 0 // Main AV channel (we = DTLS Client, camera = Server)
	IOTCChannelBack = 1 // Backchannel for Return Audio (we = DTLS Server, camera = Client)
)

const (
	BitrateMax uint16 = 0xF0 // 240 KB/s
	BitrateSD  uint16 = 0x3C // 60 KB/s
)

const (
	FrameSize1080P = 0
	FrameSize360P  = 1
	FrameSize720P  = 2
	FrameSize2K    = 3
)

const (
	QualityUnknown = 0
	QualityMax     = 1
	QualityHigh    = 2
	QualityMiddle  = 3
	QualityLow     = 4
	QualityMin     = 5
)

func CodecName(id uint16) string {
	switch id {
	case CodecH264:
		return "H264"
	case CodecH265:
		return "H265"
	case CodecMPEG4:
		return "MPEG4"
	case CodecH263:
		return "H263"
	case CodecMJPEG:
		return "MJPEG"
	default:
		return "Unknown"
	}
}

func AudioCodecName(id uint16) string {
	switch id {
	case AudioCodecG711U:
		return "PCMU"
	case AudioCodecG711A:
		return "PCMA"
	case AudioCodecPCM:
		return "PCM"
	case AudioCodecAACLATM, AudioCodecAACRaw, AudioCodecAACADTS, AudioCodecAACWyze:
		return "AAC"
	case AudioCodecOpus:
		return "Opus"
	case AudioCodecSPEEX:
		return "Speex"
	case AudioCodecMP3:
		return "MP3"
	case AudioCodecG726:
		return "G726"
	case AudioCodecADPCM:
		return "ADPCM"
	default:
		return "Unknown"
	}
}

func SampleRateValue(enum uint8) int {
	if rate, ok := SampleRates[enum]; ok {
		return rate
	}
	return 16000 // Default
}

func SampleRateIndex(hz uint32) uint8 {
	switch hz {
	case 8000:
		return SampleRate8K
	case 11025:
		return SampleRate11K
	case 12000:
		return SampleRate12K
	case 16000:
		return SampleRate16K
	case 22050:
		return SampleRate22K
	case 24000:
		return SampleRate24K
	case 32000:
		return SampleRate32K
	case 44100:
		return SampleRate44K
	case 48000:
		return SampleRate48K
	default:
		return SampleRate16K // Default
	}
}

func BuildAudioFlags(sampleRate uint32, bits16 bool, stereo bool) uint8 {
	flags := SampleRateIndex(sampleRate) << 2
	if bits16 {
		flags |= 0x02
	}
	if stereo {
		flags |= 0x01
	}
	return flags
}

func IsVideoCodec(id uint16) bool {
	return id >= CodecMPEG4 && id <= CodecH265
}

func IsAudioCodec(id uint16) bool {
	return id >= AudioCodecAACRaw && id <= AudioCodecOpus
}

func GetSamplesPerFrame(codecID uint16) uint32 {
	if samples, ok := SamplesPerFrame[codecID]; ok {
		return samples
	}
	return 1024 // Default to AAC
}
