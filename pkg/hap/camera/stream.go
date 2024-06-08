package camera

import (
	"errors"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
)

type Stream struct {
	id      string
	client  *hap.Client
	service *hap.Service
}

func NewStream(
	client *hap.Client, videoCodec *VideoCodec, audioCodec *AudioCodec,
	videoSession, audioSession *srtp.Session, bitrate int,
) (*Stream, error) {
	stream := &Stream{
		id:     core.RandString(16, 0),
		client: client,
	}

	if err := stream.GetFreeStream(); err != nil {
		return nil, err
	}

	if err := stream.ExchangeEndpoints(videoSession, audioSession); err != nil {
		return nil, err
	}

	if bitrate != 0 {
		bitrate /= 1024 // convert bps to kbps
	} else {
		bitrate = 4096 // default kbps for general FullHD camera
	}

	videoCodec.RTPParams = []RTPParams{
		{
			PayloadType:  99,
			SSRC:         videoSession.Local.SSRC,
			MaxBitrate:   uint16(bitrate), // iPhone query 299Kbps, iPad/AppleTV query 802Kbps
			RTCPInterval: 0.5,
			MaxMTU:       []uint16{1378},
		},
	}
	audioCodec.RTPParams = []RTPParams{
		{
			PayloadType:  110,
			SSRC:         audioSession.Local.SSRC,
			MaxBitrate:   24, // any iDevice query 24Kbps (this is OK for 16KHz and 1 channel)
			RTCPInterval: 5,

			ComfortNoisePayloadType: []uint8{13},
		},
	}
	audioCodec.ComfortNoise = []byte{0}

	config := &SelectedStreamConfig{
		Control: SessionControl{
			SessionID: stream.id,
			Command:   SessionCommandStart,
		},
		VideoCodec: *videoCodec,
		AudioCodec: *audioCodec,
	}

	if err := stream.SetStreamConfig(config); err != nil {
		return nil, err
	}

	return stream, nil
}

// GetFreeStream search free streaming service.
// Usual every HomeKit camera can stream only to two clients simultaniosly.
// So it has two similar services for streaming.
func (s *Stream) GetFreeStream() error {
	acc, err := s.client.GetFirstAccessory()
	if err != nil {
		return err
	}

	for _, srv := range acc.Services {
		for _, char := range srv.Characters {
			if char.Type == TypeStreamingStatus {
				var status StreamingStatus
				if err = char.ReadTLV8(&status); err != nil {
					return err
				}

				if status.Status == StreamingStatusAvailable {
					s.service = srv
					return nil
				}
			}
		}
	}

	return errors.New("hap: no free streams")
}

func (s *Stream) ExchangeEndpoints(videoSession, audioSession *srtp.Session) error {
	req := SetupEndpoints{
		SessionID: s.id,
		Address: Addr{
			IPVersion:    0,
			IPAddr:       videoSession.Local.Addr,
			VideoRTPPort: videoSession.Local.Port,
			AudioRTPPort: audioSession.Local.Port,
		},
		VideoCrypto: CryptoSuite{
			MasterKey:  string(videoSession.Local.MasterKey),
			MasterSalt: string(videoSession.Local.MasterSalt),
		},
		AudioCrypto: CryptoSuite{
			MasterKey:  string(audioSession.Local.MasterKey),
			MasterSalt: string(audioSession.Local.MasterSalt),
		},
	}

	char := s.service.GetCharacter(TypeSetupEndpoints)
	if err := char.Write(&req); err != nil {
		return err
	}
	if err := s.client.PutCharacters(char); err != nil {
		return err
	}

	var res SetupEndpoints
	if err := s.client.GetCharacter(char); err != nil {
		return err
	}
	if err := char.ReadTLV8(&res); err != nil {
		return err
	}

	videoSession.Remote = &srtp.Endpoint{
		Addr:       res.Address.IPAddr,
		Port:       res.Address.VideoRTPPort,
		MasterKey:  []byte(res.VideoCrypto.MasterKey),
		MasterSalt: []byte(res.VideoCrypto.MasterSalt),
		SSRC:       res.VideoSSRC[0],
	}

	audioSession.Remote = &srtp.Endpoint{
		Addr:       res.Address.IPAddr,
		Port:       res.Address.AudioRTPPort,
		MasterKey:  []byte(res.AudioCrypto.MasterKey),
		MasterSalt: []byte(res.AudioCrypto.MasterSalt),
		SSRC:       res.AudioSSRC[0],
	}

	return nil
}

func (s *Stream) SetStreamConfig(config *SelectedStreamConfig) error {
	char := s.service.GetCharacter(TypeSelectedStreamConfiguration)
	if err := char.Write(config); err != nil {
		return err
	}
	if err := s.client.PutCharacters(char); err != nil {
		return err
	}

	return s.client.GetCharacter(char)
}

func (s *Stream) Close() error {
	config := &SelectedStreamConfig{
		Control: SessionControl{
			SessionID: s.id,
			Command:   SessionCommandEnd,
		},
	}

	char := s.service.GetCharacter(TypeSelectedStreamConfiguration)
	if err := char.Write(config); err != nil {
		return err
	}
	return s.client.PutCharacters(char)
}
