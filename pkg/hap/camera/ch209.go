package camera

const TypeSelectedCameraRecordingConfiguration = "209"

type SelectedCameraRecordingConfiguration struct {
	GeneralConfig SupportedCameraRecordingConfiguration `tlv8:"1"`
	VideoConfig   SupportedVideoRecordingConfiguration  `tlv8:"2"`
	AudioConfig   SupportedAudioRecordingConfiguration  `tlv8:"3"`
}
