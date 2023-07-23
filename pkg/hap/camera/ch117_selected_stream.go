package camera

const TypeSelectedStreamConfiguration = "117"

type SelectedStreamConfig struct {
	Control     SessionControl      `tlv8:"1"`
	VideoParams SelectedVideoParams `tlv8:"2"`
	AudioParams SelectedAudioParams `tlv8:"3"`
}

const (
	SessionCommandEnd         = 0
	SessionCommandStart       = 1
	SessionCommandSuspend     = 2
	SessionCommandResume      = 3
	SessionCommandReconfigure = 4
)

type SessionControl struct {
	Session string `tlv8:"1"`
	Command byte   `tlv8:"2"`
}

type SelectedVideoParams struct {
	CodecType   byte             `tlv8:"1"` // only 0 - H264
	CodecParams VideoCodecParams `tlv8:"2"`
	VideoAttrs  VideoAttrs       `tlv8:"3"`
	RTPParams   VideoRTPParams   `tlv8:"4"`
}

type VideoRTPParams struct {
	PayloadType     uint8   `tlv8:"1"`
	SSRC            uint32  `tlv8:"2"`
	MaxBitrate      uint16  `tlv8:"3"`
	MinRTCPInterval float32 `tlv8:"4"`
	MaxMTU          uint16  `tlv8:"5"`
}

type SelectedAudioParams struct {
	CodecType    byte             `tlv8:"1"` // 2 - AAC_ELD, 3 - OPUS, 5 - AMR, 6 - AMR_WB
	CodecParams  AudioCodecParams `tlv8:"2"`
	RTPParams    AudioRTPParams   `tlv8:"3"`
	ComfortNoise uint8            `tlv8:"4"`
}

type AudioRTPParams struct {
	PayloadType             uint8   `tlv8:"1"`
	SSRC                    uint32  `tlv8:"2"`
	MaxBitrate              uint16  `tlv8:"3"`
	MinRTCPInterval         float32 `tlv8:"4"`
	ComfortNoisePayloadType uint8   `tlv8:"6"`
}
