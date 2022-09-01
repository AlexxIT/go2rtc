package camera

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"github.com/brutella/hap/rtp"
)

type Session struct {
	Offer  *rtp.SetupEndpoints
	Answer *rtp.SetupEndpointsResponse
	Config *rtp.StreamConfiguration
}

func NewSession() *Session {
	sessionID := RandomBytes(16)
	s := &Session{
		Offer: &rtp.SetupEndpoints{
			SessionId: sessionID,
			Video: rtp.CryptoSuite{
				MasterKey:  RandomBytes(16),
				MasterSalt: RandomBytes(14),
			},
			Audio: rtp.CryptoSuite{
				MasterKey:  RandomBytes(16),
				MasterSalt: RandomBytes(14),
			},
		},
		Config: &rtp.StreamConfiguration{
			Command: rtp.SessionControlCommand{
				Identifier: sessionID,
				Type:       rtp.SessionControlCommandTypeStart,
			},
			Video: rtp.VideoParameters{
				CodecType: rtp.VideoCodecType_H264,
				CodecParams: rtp.VideoCodecParameters{
					Profiles: []rtp.VideoCodecProfile{
						{Id: rtp.VideoCodecProfileMain},
					},
					Levels: []rtp.VideoCodecLevel{
						{Level: rtp.VideoCodecLevel4},
					},
					Packetizations: []rtp.VideoCodecPacketization{
						{Mode: rtp.VideoCodecPacketizationModeNonInterleaved},
					},
				},
				Attributes: rtp.VideoCodecAttributes{
					Width: 1920, Height: 1080, Framerate: 30,
				},
				RTP: rtp.RTPParams{
					PayloadType:             99,
					Ssrc:                    RandomUint32(),
					Bitrate:                 299,
					Interval:                0.5,
					ComfortNoisePayloadType: 98,
					MTU:                     0,
				},
			},
			Audio: rtp.AudioParameters{
				CodecType: rtp.AudioCodecType_AAC_ELD,
				CodecParams: rtp.AudioCodecParameters{
					Channels:   1,
					Bitrate:    rtp.AudioCodecBitrateVariable,
					Samplerate: rtp.AudioCodecSampleRate16Khz,
					PacketTime: 30,
				},
				RTP: rtp.RTPParams{
					PayloadType: 110,
					Ssrc:        RandomUint32(),
					Bitrate:     24,
					Interval:    5,
					MTU:         13,
				},
				ComfortNoise: false,
			},
		},
	}
	return s
}

func (s *Session) SetLocalEndpoint(host string, port uint16) {
	s.Offer.ControllerAddr = rtp.Addr{
		IPAddr:       host,
		VideoRtpPort: port,
		AudioRtpPort: port,
	}
}

func (s *Session) SetVideo() {

}

func RandomBytes(size int) []byte {
	data := make([]byte, size)
	_, _ = cryptorand.Read(data)
	return data
}

func RandomUint32() uint32 {
	data := make([]byte, 4)
	_, _ = cryptorand.Read(data)
	return binary.BigEndian.Uint32(data)
}
