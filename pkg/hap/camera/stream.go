package camera

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/srtp"
	"github.com/google/uuid"
)

type Stream struct {
	id      string
	client  *hap.Client
	service *hap.Service
}

func NewStream(
	client *hap.Client, videoCodec *VideoCodecConfiguration, audioCodec *AudioCodecConfiguration,
	videoSession, audioSession *srtp.Session, bitrate int,
) (*Stream, error) {
	u := uuid.New()
	stream := &Stream{
		id:     string(u[:]),
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

	config := &SelectedStreamConfiguration{
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
	req := SetupEndpointsRequest{
		SessionID: s.id,
		Address: Address{
			IPVersion:    0,
			IPAddr:       videoSession.Local.Addr,
			VideoRTPPort: videoSession.Local.Port,
			AudioRTPPort: audioSession.Local.Port,
		},
		VideoCrypto: SRTPCryptoSuite{
			CryptoSuite: CryptoAES_CM_128_HMAC_SHA1_80,
			MasterKey:   string(videoSession.Local.MasterKey),
			MasterSalt:  string(videoSession.Local.MasterSalt),
		},
		AudioCrypto: SRTPCryptoSuite{
			CryptoSuite: CryptoAES_CM_128_HMAC_SHA1_80,
			MasterKey:   string(audioSession.Local.MasterKey),
			MasterSalt:  string(audioSession.Local.MasterSalt),
		},
	}

	log.Printf("[hap] SetupEndpointsRequest: SessionID=%x IPAddr=%s VideoPort=%d AudioPort=%d VideoKeyLen=%d AudioKeyLen=%d CryptoSuite=%d IPVersion=%d",
		req.SessionID, req.Address.IPAddr, req.Address.VideoRTPPort, req.Address.AudioRTPPort, len(req.VideoCrypto.MasterKey), len(req.AudioCrypto.MasterKey), req.VideoCrypto.CryptoSuite, req.Address.IPVersion)

	// Hex dump the marshalled TLV8 for debugging
	debugBytes, debugErr := tlv8.Marshal(&req)
	if debugErr != nil {
		log.Printf("[hap] TLV8 marshal debug error: %v", debugErr)
	} else {
		log.Printf("[hap] SetupEndpointsRequest TLV8 hex (%d bytes): %x", len(debugBytes), debugBytes)
		log.Printf("[hap] SetupEndpointsRequest TLV8 base64: %s", base64.StdEncoding.EncodeToString(debugBytes))
	}

	char := s.service.GetCharacter(TypeSetupEndpoints)
	if err := char.Write(&req); err != nil {
		return err
	}
	if err := s.client.PutCharacters(char); err != nil {
		return err
	}

	var res SetupEndpointsResponse
	if err := s.client.GetCharacter(char); err != nil {
		return err
	}
	if sVal, ok := char.Value.(string); ok {
		log.Printf("[hap] SetupEndpointsResponse raw base64: %s", sVal)
	}

	if err := char.ReadTLV8(&res); err != nil {
		return err
	}
	if res.Status != StreamingStatusAvailable {
		return fmt.Errorf("hap: setup endpoints rejected with status %d", res.Status)
	}
	if len(res.VideoCrypto.MasterKey) != 16 || len(res.VideoCrypto.MasterSalt) != 14 ||
		len(res.AudioCrypto.MasterKey) != 16 || len(res.AudioCrypto.MasterSalt) != 14 {
		return fmt.Errorf("hap: setup endpoints returned invalid crypto lengths video=%d/%d audio=%d/%d",
			len(res.VideoCrypto.MasterKey), len(res.VideoCrypto.MasterSalt),
			len(res.AudioCrypto.MasterKey), len(res.AudioCrypto.MasterSalt))
	}

	cameraIP, _, _ := net.SplitHostPort(s.client.DeviceAddress)
	addr := res.Address.IPAddr
	if addr == "0.0.0.0" || addr == "" {
		addr = cameraIP
	}

	log.Printf("[hap] ExchangeEndpoints VideoCrypto key=%d salt=%d AudioCrypto key=%d salt=%d",
		len(res.VideoCrypto.MasterKey), len(res.VideoCrypto.MasterSalt),
		len(res.AudioCrypto.MasterKey), len(res.AudioCrypto.MasterSalt))

	videoSession.Remote = &srtp.Endpoint{
		Addr:       addr,
		Port:       res.Address.VideoRTPPort,
		MasterKey:  []byte(res.VideoCrypto.MasterKey),
		MasterSalt: []byte(res.VideoCrypto.MasterSalt),
		SSRC:       res.VideoSSRC,
	}

	audioSession.Remote = &srtp.Endpoint{
		Addr:       addr,
		Port:       res.Address.AudioRTPPort,
		MasterKey:  []byte(res.AudioCrypto.MasterKey),
		MasterSalt: []byte(res.AudioCrypto.MasterSalt),
		SSRC:       res.AudioSSRC,
	}

	return nil
}

func (s *Stream) SetStreamConfig(config *SelectedStreamConfiguration) error {
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
	config := &SelectedStreamConfiguration{
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
