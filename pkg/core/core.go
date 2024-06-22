package core

import "encoding/json"

const (
	DirectionRecvonly = "recvonly"
	DirectionSendonly = "sendonly"
	DirectionSendRecv = "sendrecv"
)

const (
	KindVideo = "video"
	KindAudio = "audio"
)

const (
	CodecH264 = "H264" // payloadType: 96
	CodecH265 = "H265"
	CodecVP8  = "VP8"
	CodecVP9  = "VP9"
	CodecAV1  = "AV1"
	CodecJPEG = "JPEG" // payloadType: 26
	CodecRAW  = "RAW"

	CodecPCMU = "PCMU" // payloadType: 0
	CodecPCMA = "PCMA" // payloadType: 8
	CodecAAC  = "MPEG4-GENERIC"
	CodecOpus = "OPUS" // payloadType: 111
	CodecG722 = "G722"
	CodecMP3  = "MPA" // payload: 14, aka MPEG-1 Layer III
	CodecPCM  = "L16" // Linear PCM (big endian)

	CodecPCML = "PCML" // Linear PCM (little endian)

	CodecELD  = "ELD" // AAC-ELD
	CodecFLAC = "FLAC"

	CodecAll = "ALL"
	CodecAny = "ANY"
)

const PayloadTypeRAW byte = 255

type Producer interface {
	// GetMedias - return Media(s) with local Media.Direction:
	// - recvonly for Producer Video/Audio
	// - sendonly for Producer backchannel
	GetMedias() []*Media

	// GetTrack - return Receiver, that can only produce rtp.Packet(s)
	GetTrack(media *Media, codec *Codec) (*Receiver, error)

	// Deprecated: rename to Run()
	Start() error

	// Deprecated: rename to Close()
	Stop() error
}

type Consumer interface {
	// GetMedias - return Media(s) with local Media.Direction:
	// - sendonly for Consumer Video/Audio
	// - recvonly for Consumer backchannel
	GetMedias() []*Media

	AddTrack(media *Media, codec *Codec, track *Receiver) error

	// Deprecated: rename to Close()
	Stop() error
}

type Mode byte

const (
	ModeActiveProducer Mode = iota + 1 // typical source (client)
	ModePassiveConsumer
	ModePassiveProducer
	ModeActiveConsumer
)

func (m Mode) String() string {
	switch m {
	case ModeActiveProducer:
		return "active producer"
	case ModePassiveConsumer:
		return "passive consumer"
	case ModePassiveProducer:
		return "passive producer"
	case ModeActiveConsumer:
		return "active consumer"
	}
	return "unknown"
}

func (m Mode) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}
