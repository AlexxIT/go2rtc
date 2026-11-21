package camera

// SupportedVideoStreamTiers describes multi-tier RTP/WebRTC video encodings
// (UUID 8043 / 8059)
type SupportedVideoStreamTiers struct {
	Codec       byte              `tlv8:"1"`
	PayloadType uint8             `tlv8:"2"`
	Tiers       []VideoStreamTier `tlv8:"3"`
}

// VideoStreamTier is one quality tier for a video codec
type VideoStreamTier struct {
	Identifier           uint32 `tlv8:"1"`
	Quality              byte   `tlv8:"2"`
	TargetAverageBitrate uint32 `tlv8:"3"` // kbps
	Width                uint16 `tlv8:"4"`
	Height               uint16 `tlv8:"5"`
	FrameRate            uint8  `tlv8:"6"`
}

// SupportedAudioStreamTiers describes multi-tier audio encodings
// (UUID 8044 / 805A)
type SupportedAudioStreamTiers struct {
	Codec       byte              `tlv8:"1"`
	PayloadType uint8             `tlv8:"2"`
	Tiers       []AudioStreamTier `tlv8:"3"`
}

// AudioStreamTier is one quality tier for an audio codec
// Spec currently allows exactly one tier
type AudioStreamTier struct {
	Identifier           uint32 `tlv8:"1"`
	TargetAverageBitrate uint32 `tlv8:"2"` // bits per second
	SampleRate           byte   `tlv8:"3"`
	BitDepth             byte   `tlv8:"4"`
	PacketTime           uint8  `tlv8:"5"` // only 20 ms allowed
	NumberOfChannels     uint8  `tlv8:"6"` // only 1 allowed
}

// Default1080pVideoTiers returns High/Medium/Low tiers for a 1080p 16:9 sensor
func Default1080pVideoTiers(codec byte, payloadType uint8) SupportedVideoStreamTiers {
	return SupportedVideoStreamTiers{
		Codec:       codec,
		PayloadType: payloadType,
		Tiers: []VideoStreamTier{
			{
				Identifier:           1,
				Quality:              VideoQualityHigh,
				TargetAverageBitrate: Bitrate1080pAvgKbps,
				Width:                1920,
				Height:               1080,
				FrameRate:            30,
			},
			{
				Identifier:           2,
				Quality:              VideoQualityMedium,
				TargetAverageBitrate: Bitrate720pAvgKbps,
				Width:                1280,
				Height:               720,
				FrameRate:            30,
			},
			{
				Identifier:           3,
				Quality:              VideoQualityLow,
				TargetAverageBitrate: BitrateLowAvgKbps,
				Width:                640,
				Height:               360,
				FrameRate:            15,
			},
		},
	}
}

// DefaultOpusAudioTier returns the mandatory Opus audio tier (16 kHz capture,
// reported transmission sample rate 48 kHz per the guide note)
func DefaultOpusAudioTier(payloadType uint8) SupportedAudioStreamTiers {
	return SupportedAudioStreamTiers{
		Codec:       AudioCodecTypeOpus,
		PayloadType: payloadType,
		Tiers: []AudioStreamTier{
			{
				Identifier:           1,
				TargetAverageBitrate: 24000,
				SampleRate:           AudioTierSampleRate48kHz,
				BitDepth:             AudioTierBitDepth16,
				PacketTime:           20,
				NumberOfChannels:     1,
			},
		},
	}
}
