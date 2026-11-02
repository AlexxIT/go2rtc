package camera

const TypeSupportedVideoRecordingConfiguration = "206"

type SupportedVideoRecordingConfiguration struct {
	CodecConfigs []VideoRecordingCodecConfiguration `tlv8:"1"`
}

type VideoRecordingCodecConfiguration struct {
	CodecType   uint8                         `tlv8:"1"`
	CodecParams VideoRecordingCodecParameters `tlv8:"2"`
	// Slice, not a single struct: the controller may send several, and a
	// one-element slice encodes to identical bytes so the wire format for
	// what we advertise is unchanged.
	CodecAttrs []VideoCodecAttributes `tlv8:"3"`
}

// ProfileID and Level are lists of supported values, like the live-view
// VideoCodecParameters - see HAP-NodeJS's
// RecordingManagement._supportedVideoRecordingConfiguration, which encodes
// them from `profiles`/`levels` arrays.
//
// Bitrate and IFrameInterval are slices so they encode to nothing when empty.
// They are "only present in the Selected Camera Recording Configuration
// request" (secure-video-specification 3.9): if the accessory includes them
// when advertising its supported configurations, the controller rejects the
// whole characteristic, never writes a Selected Configuration, and never
// opens a DataStream - so recording silently never starts.
type VideoRecordingCodecParameters struct {
	ProfileID      []byte   `tlv8:"1"`
	Level          []byte   `tlv8:"2"`
	Bitrate        []uint32 `tlv8:"3"`
	IFrameInterval []uint32 `tlv8:"4"`
}
