package core

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

type Info struct {
	Type       string      `json:"type,omitempty"`
	URL        string      `json:"url,omitempty"`
	RemoteAddr string      `json:"remote_addr,omitempty"`
	UserAgent  string      `json:"user_agent,omitempty"`
	SDP        string      `json:"sdp,omitempty"`
	Medias     []*Media    `json:"medias,omitempty"`
	Receivers  []*Receiver `json:"receivers,omitempty"`
	Senders    []*Sender   `json:"senders,omitempty"`
	Recv       int         `json:"recv,omitempty"`
	Send       int         `json:"send,omitempty"`
}

const (
	UnsupportedCodec    = "unsupported codec"
	WrongMediaDirection = "wrong media direction"
)

type SuperProducer struct {
	Type      string      `json:"type,omitempty"`
	URL       string      `json:"url,omitempty"`
	Medias    []*Media    `json:"medias,omitempty"`
	Receivers []*Receiver `json:"receivers,omitempty"`
	Recv      int         `json:"recv,omitempty"`
}

func (s *SuperProducer) GetMedias() []*Media {
	return s.Medias
}

func (s *SuperProducer) GetTrack(media *Media, codec *Codec) (*Receiver, error) {
	for _, receiver := range s.Receivers {
		if receiver.Codec == codec {
			return receiver, nil
		}
	}
	receiver := NewReceiver(media, codec)
	s.Receivers = append(s.Receivers, receiver)
	return receiver, nil
}

func (s *SuperProducer) Close() error {
	for _, receiver := range s.Receivers {
		receiver.Close()
	}
	return nil
}

type SuperConsumer struct {
	Type       string    `json:"type,omitempty"`
	URL        string    `json:"url,omitempty"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Medias     []*Media  `json:"medias,omitempty"`
	Senders    []*Sender `json:"receivers,omitempty"`
	Send       int       `json:"recv,omitempty"`
}

func (s *SuperConsumer) GetMedias() []*Media {
	return s.Medias
}

func (s *SuperConsumer) AddTrack(media *Media, codec *Codec, track *Receiver) error {
	return nil
}

//func (b *SuperConsumer) WriteTo(w io.Writer) (n int64, err error) {
//	return 0, nil
//}

func (s *SuperConsumer) Close() error {
	for _, sender := range s.Senders {
		sender.Close()
	}
	return nil
}

func (s *SuperConsumer) Codecs() []*Codec {
	codecs := make([]*Codec, len(s.Senders))
	for i, sender := range s.Senders {
		codecs[i] = sender.Codec
	}
	return codecs
}
