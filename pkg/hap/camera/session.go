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

func NewSession(vp *rtp.VideoParameters, ap *rtp.AudioParameters) *Session {
	vp.RTP = rtp.RTPParams{
		PayloadType: 99,
		Ssrc:        RandomUint32(),
		Bitrate:     2048,
		Interval:    10,
		MTU:         1200, // like WebRTC
	}
	ap.RTP = rtp.RTPParams{
		PayloadType:             110,
		Ssrc:                    RandomUint32(),
		Bitrate:                 32,
		Interval:                10,
		ComfortNoisePayloadType: 98,
		MTU:                     0,
	}

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
			Video: *vp,
			Audio: *ap,
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
