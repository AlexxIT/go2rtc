package camera

const TypeSelectedStreamConfiguration = "117"

type SelectedStreamConfig struct {
	Control    SessionControl `tlv8:"1"`
	VideoCodec VideoCodec     `tlv8:"2"`
	AudioCodec AudioCodec     `tlv8:"3"`
}

//goland:noinspection ALL
const (
	SessionCommandEnd         = 0
	SessionCommandStart       = 1
	SessionCommandSuspend     = 2
	SessionCommandResume      = 3
	SessionCommandReconfigure = 4
)

type SessionControl struct {
	SessionID string `tlv8:"1"`
	Command   byte   `tlv8:"2"`
}

type RTPParams struct {
	PayloadType             uint8    `tlv8:"1"`
	SSRC                    uint32   `tlv8:"2"`
	MaxBitrate              uint16   `tlv8:"3"`
	RTCPInterval            float32  `tlv8:"4"`
	MaxMTU                  []uint16 `tlv8:"5"`
	ComfortNoisePayloadType []uint8  `tlv8:"6"`
}
