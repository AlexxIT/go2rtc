package camera

const TypeSupportedVideoRecordingConfiguration = "206"

type SupportedVideoRecordingConfiguration struct {
	CodecConfigs []VideoRecordingCodecConfiguration `tlv8:"1"`
}

type VideoRecordingCodecConfiguration struct {
	CodecType   uint8                         `tlv8:"1"`
	CodecParams VideoRecordingCodecParameters `tlv8:"2"`
	CodecAttrs  VideoCodecAttributes          `tlv8:"3"`
}

type VideoRecordingCodecParameters struct {
	ProfileID      uint8  `tlv8:"1"`
	Level          uint8  `tlv8:"2"`
	Bitrate        uint32 `tlv8:"3"`
	IFrameInterval uint32 `tlv8:"4"`
}
