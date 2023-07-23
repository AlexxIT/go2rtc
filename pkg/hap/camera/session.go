package camera

import (
	"crypto/rand"
	"encoding/binary"
)

type Session struct {
	Offer  *SetupEndpoints
	Answer *SetupEndpointsResponse
	Config *SelectedStreamConfig
}

func NewSession(vp *SelectedVideoParams, ap *SelectedAudioParams) *Session {
	vp.RTPParams = VideoRTPParams{
		PayloadType:     99,
		SSRC:            RandomUint32(),
		MaxBitrate:      2048,
		MinRTCPInterval: 10,
		MaxMTU:          1200, // like WebRTC
	}
	ap.RTPParams = AudioRTPParams{
		PayloadType:             110,
		SSRC:                    RandomUint32(),
		MaxBitrate:              32,
		MinRTCPInterval:         10,
		ComfortNoisePayloadType: 98,
	}

	sessionID := RandomBytes(16)
	s := &Session{
		Offer: &SetupEndpoints{
			SessionID: sessionID,
			VideoCrypto: CryptoSuite{
				MasterKey:  RandomBytes(16),
				MasterSalt: RandomBytes(14),
			},
			AudioCrypto: CryptoSuite{
				MasterKey:  RandomBytes(16),
				MasterSalt: RandomBytes(14),
			},
		},
		Config: &SelectedStreamConfig{
			Control: SessionControl{
				Session: string(sessionID),
				Command: SessionCommandStart,
			},
			VideoParams: *vp,
			AudioParams: *ap,
		},
	}
	return s
}

func (s *Session) SetLocalEndpoint(host string, port uint16) {
	s.Offer.ControllerAddr = Addr{
		IPAddr:       host,
		VideoRTPPort: port,
		AudioRTPPort: port,
	}
}

func RandomBytes(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

func RandomUint32() uint32 {
	data := make([]byte, 4)
	_, _ = rand.Read(data)
	return binary.BigEndian.Uint32(data)
}
