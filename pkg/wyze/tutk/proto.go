package tutk

type AVLoginResponse struct {
	ServerType      uint32
	Resend          int32
	TwoWayStreaming int32
	SyncRecvData    int32
	SecurityMode    uint32
	VideoOnConnect  int32
	AudioOnConnect  int32
}

const (
	CodecUnknown uint16 = 0x00
	CodecMPEG4   uint16 = 0x4C // 76
	CodecH263    uint16 = 0x4D // 77
	CodecH264    uint16 = 0x4E // 78
	CodecMJPEG   uint16 = 0x4F // 79
	CodecH265    uint16 = 0x50 // 80
)

const (
	AudioCodecAACRaw  uint16 = 0x86 // 134
	AudioCodecAACADTS uint16 = 0x87 // 135
	AudioCodecAACLATM uint16 = 0x88 // 136
	AudioCodecG711U   uint16 = 0x89 // 137
	AudioCodecG711A   uint16 = 0x8A // 138
	AudioCodecADPCM   uint16 = 0x8B // 139
	AudioCodecPCM     uint16 = 0x8C // 140
	AudioCodecSPEEX   uint16 = 0x8D // 141
	AudioCodecMP3     uint16 = 0x8E // 142
	AudioCodecG726    uint16 = 0x8F // 143
	AudioCodecAACWyze uint16 = 0x90 // 144
	AudioCodecOpus    uint16 = 0x92 // 146
)

const (
	SampleRate8K  uint8 = 0x00
	SampleRate11K uint8 = 0x01
	SampleRate12K uint8 = 0x02
	SampleRate16K uint8 = 0x03
	SampleRate22K uint8 = 0x04
	SampleRate24K uint8 = 0x05
	SampleRate32K uint8 = 0x06
	SampleRate44K uint8 = 0x07
	SampleRate48K uint8 = 0x08
)

var sampleRates = map[uint8]int{
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

var samplesPerFrame = map[uint16]uint32{
	AudioCodecAACRaw:  1024,
	AudioCodecAACADTS: 1024,
	AudioCodecAACLATM: 1024,
	AudioCodecAACWyze: 1024,
	AudioCodecG711U:   160,
	AudioCodecG711A:   160,
	AudioCodecPCM:     160,
	AudioCodecADPCM:   160,
	AudioCodecSPEEX:   160,
	AudioCodecMP3:     1152,
	AudioCodecG726:    160,
	AudioCodecOpus:    960,
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

// OLD Protocol (IOTC/TransCode)
const (
	CmdDiscoReq     uint16 = 0x0601
	CmdDiscoRes     uint16 = 0x0602
	CmdSessionReq   uint16 = 0x0402
	CmdSessionRes   uint16 = 0x0404
	CmdDataTX       uint16 = 0x0407
	CmdDataRX       uint16 = 0x0408
	CmdKeepaliveReq uint16 = 0x0427
	CmdKeepaliveRes uint16 = 0x0428

	OldHeaderSize    = 16
	OldDiscoBodySize = 72
	OldDiscoSize     = OldHeaderSize + OldDiscoBodySize
	OldSessionBody   = 36
	OldSessionSize   = OldHeaderSize + OldSessionBody
)

// NEW Protocol (0xCC51)
const (
	MagicNewProto    uint16 = 0xCC51
	CmdNewDisco      uint16 = 0x1002
	CmdNewKeepalive  uint16 = 0x1202
	CmdNewClose      uint16 = 0x1302
	CmdNewDTLS       uint16 = 0x1502
	NewPayloadSize   uint16 = 0x0028
	NewPacketSize           = 52
	NewHeaderSize           = 28
	NewAuthSize             = 20
	NewKeepaliveSize        = 48
)

const (
	UIDSize    = 20
	RandIDSize = 8
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
	ProtoVersion uint16 = 0x000c
	DefaultCaps  uint32 = 0x001f07fb
)

const (
	IOTCChannelMain = 0 // Main AV (we = DTLS Client)
	IOTCChannelBack = 1 // Backchannel (we = DTLS Server)
)

const (
	PSKIdentity = "AUTHPWD_admin"
	DefaultUser = "admin"
	DefaultPort = 32761
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

func SampleRateValue(idx uint8) int {
	if rate, ok := sampleRates[idx]; ok {
		return rate
	}
	return 16000
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
		return SampleRate16K
	}
}

func BuildAudioFlags(sampleRate uint32, bits16, stereo bool) uint8 {
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
	if samples, ok := samplesPerFrame[codecID]; ok {
		return samples
	}
	return 1024
}
