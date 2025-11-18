package camera

const TypeSupportedCameraRecordingConfiguration = "205"

type SupportedCameraRecordingConfiguration struct {
	PrebufferLength              uint32 `tlv8:"1"`
	EventTriggerOptions          uint64 `tlv8:"2"`
	MediaContainerConfigurations `tlv8:"3"`
}

type MediaContainerConfigurations struct {
	MediaContainerType       uint8 `tlv8:"1"`
	MediaContainerParameters `tlv8:"2"`
}

type MediaContainerParameters struct {
	FragmentLength uint32 `tlv8:"1"`
}
