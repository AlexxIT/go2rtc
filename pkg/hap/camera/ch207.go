package camera

const TypeSupportedAudioRecordingConfiguration = "207"

type SupportedAudioRecordingConfiguration struct {
	CodecConfigs []AudioRecordingCodecConfiguration `tlv8:"1"`
}

type AudioRecordingCodecConfiguration struct {
	CodecType   byte                            `tlv8:"1"`
	CodecParams []AudioRecordingCodecParameters `tlv8:"2"`
}

type AudioRecordingCodecParameters struct {
	Channels        uint8    `tlv8:"1"`
	BitrateMode     []byte   `tlv8:"2"`
	SampleRate      []byte   `tlv8:"3"`
	MaxAudioBitrate []uint32 `tlv8:"4"`
}
