package camera

const TypeSupportedAudioStreamConfiguration = "115"

type SupportedAudioStreamConfig struct {
	Codecs       []AudioCodecConfig `tlv8:"1"`
	ComfortNoise byte               `tlv8:"2"`
}

const (
	AudioCodecTypePCMU   = 0
	AudioCodecTypePCMA   = 1
	AudioCodecTypeAACELD = 2
	AudioCodecTypeOpus   = 3
	AudioCodecTypeMSBC   = 4
	AudioCodecTypeAMR    = 5
	AudioCodecTypeARMWB  = 6

	AudioCodecBitrateVariable = 0
	AudioCodecBitrateConstant = 1

	AudioCodecSampleRate8Khz  = 0
	AudioCodecSampleRate16Khz = 1
	AudioCodecSampleRate24Khz = 2
)

type AudioCodecConfig struct {
	CodecType   byte               `tlv8:"1"`
	CodecParams []AudioCodecParams `tlv8:"2"`
}

type AudioCodecParams struct {
	Channels   byte `tlv8:"1"`
	Bitrate    byte `tlv8:"2"` // 0 - variable, 1 - constant
	SampleRate byte `tlv8:"3"` // 0 - 8000, 1 - 16000, 2 - 24000
	RTPTime    byte `tlv8:"4"`
}
