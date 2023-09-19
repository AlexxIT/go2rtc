package camera

const TypeSupportedVideoStreamConfiguration = "114"

type SupportedVideoStreamConfig struct {
	Codecs []VideoCodec `tlv8:"1"`
}

type VideoCodec struct {
	CodecType   byte          `tlv8:"1"`
	CodecParams []VideoParams `tlv8:"2"`
	VideoAttrs  []VideoAttrs  `tlv8:"3"`
	RTPParams   []RTPParams   `tlv8:"4"`
}

//goland:noinspection ALL
const (
	VideoCodecTypeH264 = 0

	VideoCodecProfileConstrainedBaseline = 0
	VideoCodecProfileMain                = 1
	VideoCodecProfileHigh                = 2

	VideoCodecLevel31 = 0
	VideoCodecLevel32 = 1
	VideoCodecLevel40 = 2

	VideoCodecPacketizationModeNonInterleaved = 0

	VideoCodecCvoNotSuppported = 0
	VideoCodecCvoSuppported    = 1
)

type VideoParams struct {
	ProfileID         []byte `tlv8:"1"` // 0 - baseline, 1 - main, 2 - high
	Level             []byte `tlv8:"2"` // 0 - 3.1, 1 - 3.2, 2 - 4.0
	PacketizationMode byte   `tlv8:"3"` // only 0 - non interleaved
	CVOEnabled        []byte `tlv8:"4"` // 0 - not supported, 1 - supported
	CVOID             []byte `tlv8:"5"` // ???
}

type VideoAttrs struct {
	Width     uint16 `tlv8:"1"`
	Height    uint16 `tlv8:"2"`
	Framerate uint8  `tlv8:"3"`
}
