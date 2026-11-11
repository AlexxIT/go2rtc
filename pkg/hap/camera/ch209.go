package camera

const TypeSelectedCameraRecordingConfiguration = "209"

// The controller sends exactly one chosen video/audio config here, as that
// config's fields directly (CodecType, CodecParams, ...) - not wrapped in
// the CodecConfigs array the Supported characteristic uses to advertise
// multiple options. Reusing the Supported* wrapper types here made the
// decoder treat the flat CodecType byte as if it were the array wrapper,
// producing a "wrong size" error on every controller write.
type SelectedCameraRecordingConfiguration struct {
	GeneralConfig SupportedCameraRecordingConfiguration `tlv8:"1"`
	VideoConfig   VideoRecordingCodecConfiguration      `tlv8:"2"`
	AudioConfig   AudioRecordingCodecConfiguration      `tlv8:"3"`
}
