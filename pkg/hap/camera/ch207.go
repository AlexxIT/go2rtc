package camera

const TypeSupportedAudioRecordingConfiguration = "207"

//goland:noinspection ALL
const (
	// Recording uses its own audio codec enum, which is NOT the streaming
	// AudioCodecType* enum from ch115 (where AAC-ELD is 2 and 3 is Opus).
	// See secure-video-specification 3.10 and HAP-NodeJS's
	// AudioRecordingCodecType.
	AudioRecordingCodecTypeAACLC  = 0
	AudioRecordingCodecTypeAACELD = 1

	AudioRecordingSampleRate8Khz  = 0
	AudioRecordingSampleRate16Khz = 1
	AudioRecordingSampleRate24Khz = 2
	AudioRecordingSampleRate32Khz = 3
	AudioRecordingSampleRate44Khz = 4
	AudioRecordingSampleRate48Khz = 5
)

type SupportedAudioRecordingConfiguration struct {
	CodecConfigs []AudioRecordingCodecConfiguration `tlv8:"1"`
}

type AudioRecordingCodecConfiguration struct {
	CodecType   byte                            `tlv8:"1"`
	CodecParams []AudioRecordingCodecParameters `tlv8:"2"`
}

// MaxAudioBitrate, like the video Bitrate/IFrameInterval, is only present in
// the Selected Camera Recording Configuration - leave it empty when
// advertising supported configurations.
type AudioRecordingCodecParameters struct {
	Channels        uint8    `tlv8:"1"`
	BitrateMode     []byte   `tlv8:"2"`
	SampleRate      []byte   `tlv8:"3"`
	MaxAudioBitrate []uint32 `tlv8:"4"`
}
