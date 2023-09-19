package camera

const TypeSupportedAudioStreamConfiguration = "115"

type SupportedAudioStreamConfig struct {
	Codecs       []AudioCodec `tlv8:"1"`
	ComfortNoise byte         `tlv8:"2"`
}

//goland:noinspection ALL
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

	RTPTimeAACELD8  = 60 // 8000/1000*60=480
	RTPTimeAACELD16 = 30 // 16000/1000*30=480
	RTPTimeAACELD24 = 20 // 24000/1000*20=480
	RTPTimeAACLD16  = 60 // 16000/1000*60=960
	RTPTimeAACLD24  = 40 // 24000/1000*40=960
)

type AudioCodec struct {
	CodecType    byte          `tlv8:"1"`
	CodecParams  []AudioParams `tlv8:"2"`
	RTPParams    []RTPParams   `tlv8:"3"`
	ComfortNoise []byte        `tlv8:"4"`
}

type AudioParams struct {
	Channels   uint8   `tlv8:"1"`
	Bitrate    byte    `tlv8:"2"` // 0 - variable, 1 - constant
	SampleRate []byte  `tlv8:"3"` // 0 - 8000, 1 - 16000, 2 - 24000
	RTPTime    []uint8 `tlv8:"4"` // 20, 30, 40, 60
}
