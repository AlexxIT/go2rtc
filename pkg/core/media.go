package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pion/sdp/v3"
)

// Media take best from:
// - deepch/vdk/format/rtsp/sdp.Media
// - pion/sdp.MediaDescription
type Media struct {
	Kind      string   `json:"kind,omitempty"`      // video or audio
	Direction string   `json:"direction,omitempty"` // sendonly, recvonly
	Codecs    []*Codec `json:"codecs,omitempty"`

	ID string `json:"id,omitempty"` // MID for WebRTC, Control for RTSP
}

func (m *Media) String() string {
	s := fmt.Sprintf("%s, %s", m.Kind, m.Direction)
	for _, codec := range m.Codecs {
		name := codec.String()

		if strings.Contains(s, name) {
			continue
		}

		s += ", " + name
	}
	return s
}

func (m *Media) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

func (m *Media) Clone() *Media {
	clone := *m
	clone.Codecs = make([]*Codec, len(m.Codecs))
	for i, codec := range m.Codecs {
		clone.Codecs[i] = codec.Clone()
	}
	return &clone
}

func (m *Media) MatchMedia(remote *Media) (codec, remoteCodec *Codec) {
	// check same kind and opposite dirrection
	if m.Kind != remote.Kind ||
		m.Direction == DirectionSendonly && remote.Direction != DirectionRecvonly ||
		m.Direction == DirectionRecvonly && remote.Direction != DirectionSendonly {
		return nil, nil
	}

	for _, codec = range m.Codecs {
		for _, remoteCodec = range remote.Codecs {
			if codec.Match(remoteCodec) {
				return
			}
		}
	}

	return nil, nil
}

func (m *Media) MatchCodec(remote *Codec) *Codec {
	for _, codec := range m.Codecs {
		if codec.Match(remote) {
			return codec
		}
	}
	return nil
}

func (m *Media) MatchAll() bool {
	for _, codec := range m.Codecs {
		if codec.Name == CodecAll {
			return true
		}
	}
	return false
}

func (m *Media) Equal(media *Media) bool {
	if media.ID != "" {
		return m.ID == media.ID
	}
	return m.String() == media.String()
}

func GetKind(name string) string {
	switch name {
	case CodecH264, CodecH265, CodecVP8, CodecVP9, CodecAV1, CodecJPEG, CodecRAW:
		return KindVideo
	case CodecPCMU, CodecPCMA, CodecAAC, CodecOpus, CodecG722, CodecMP3, CodecPCM, CodecPCML, CodecELD, CodecFLAC:
		return KindAudio
	}
	return ""
}

func MarshalSDP(name string, medias []*Media) ([]byte, error) {
	sd := &sdp.SessionDescription{
		Origin: sdp.Origin{
			Username: "-", SessionID: 1, SessionVersion: 1,
			NetworkType: "IN", AddressType: "IP4", UnicastAddress: "0.0.0.0",
		},
		SessionName: sdp.SessionName(name),
		ConnectionInformation: &sdp.ConnectionInformation{
			NetworkType: "IN", AddressType: "IP4", Address: &sdp.Address{
				Address: "0.0.0.0",
			},
		},
		TimeDescriptions: []sdp.TimeDescription{
			{Timing: sdp.Timing{}},
		},
	}

	for _, media := range medias {
		if media.Codecs == nil {
			continue
		}

		codec := media.Codecs[0]

		name := codec.Name
		if name == CodecELD {
			name = CodecAAC
		}

		md := &sdp.MediaDescription{
			MediaName: sdp.MediaName{
				Media:  media.Kind,
				Protos: []string{"RTP", "AVP"},
			},
		}
		md.WithCodec(codec.PayloadType, name, codec.ClockRate, codec.Channels, codec.FmtpLine)

		if media.ID != "" {
			md.WithValueAttribute("control", media.ID)
		}

		sd.MediaDescriptions = append(sd.MediaDescriptions, md)
	}

	return sd.Marshal()
}

func UnmarshalMedia(md *sdp.MediaDescription) *Media {
	m := &Media{
		Kind: md.MediaName.Media,
	}

	for _, attr := range md.Attributes {
		switch attr.Key {
		case DirectionSendonly, DirectionRecvonly, DirectionSendRecv:
			m.Direction = attr.Key
		case "control", "mid":
			m.ID = attr.Value
		}
	}

	for _, format := range md.MediaName.Formats {
		m.Codecs = append(m.Codecs, UnmarshalCodec(md, format))
	}

	return m
}

func ParseQuery(query map[string][]string) (medias []*Media) {
	// set media candidates from query list
	for key, values := range query {
		switch key {
		case KindVideo, KindAudio:
			for _, value := range values {
				media := &Media{Kind: key, Direction: DirectionSendonly}

				for _, name := range strings.Split(value, ",") {
					name = strings.ToUpper(name)

					// check aliases
					switch name {
					case "", "COPY":
						name = CodecAny
					case "MJPEG":
						name = CodecJPEG
					case "AAC":
						name = CodecAAC
					case "MP3":
						name = CodecMP3
					}

					media.Codecs = append(media.Codecs, &Codec{Name: name})
				}

				medias = append(medias, media)
			}
		}
	}

	return
}
