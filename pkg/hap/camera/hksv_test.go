package camera

import (
	"encoding/base64"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/stretchr/testify/require"
)

func TestDefault1080pVideoTiers(t *testing.T) {
	tiers := Default1080pVideoTiers(VideoCodecTypeTierH265, 100)
	require.Equal(t, byte(VideoCodecTypeTierH265), tiers.Codec)
	require.Equal(t, uint8(100), tiers.PayloadType)
	require.Len(t, tiers.Tiers, 3)
	require.Equal(t, byte(VideoQualityHigh), tiers.Tiers[0].Quality)
	require.Equal(t, uint16(1920), tiers.Tiers[0].Width)
	require.Equal(t, uint16(1080), tiers.Tiers[0].Height)
	require.Equal(t, uint32(Bitrate1080pAvgKbps), tiers.Tiers[0].TargetAverageBitrate)
	require.Equal(t, byte(VideoQualityMedium), tiers.Tiers[1].Quality)
	require.Equal(t, byte(VideoQualityLow), tiers.Tiers[2].Quality)
	require.Equal(t, uint8(15), tiers.Tiers[2].FrameRate)

	b, err := tlv8.Marshal(tiers)
	require.NoError(t, err)
	require.NotEmpty(t, b)

	var out SupportedVideoStreamTiers
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.Equal(t, tiers.Codec, out.Codec)
	require.Equal(t, tiers.PayloadType, out.PayloadType)
	require.Len(t, out.Tiers, 3)
	require.Equal(t, tiers.Tiers[0].Width, out.Tiers[0].Width)
	require.Equal(t, tiers.Tiers[2].Height, out.Tiers[2].Height)
}

func TestDefaultOpusAudioTier(t *testing.T) {
	audio := DefaultOpusAudioTier(111)
	require.Equal(t, byte(AudioCodecTypeOpus), audio.Codec)
	require.Len(t, audio.Tiers, 1)
	require.Equal(t, uint8(20), audio.Tiers[0].PacketTime)
	require.Equal(t, uint8(1), audio.Tiers[0].NumberOfChannels)
	require.Equal(t, byte(AudioTierSampleRate48kHz), audio.Tiers[0].SampleRate)

	b, err := tlv8.Marshal(audio)
	require.NoError(t, err)

	var out SupportedAudioStreamTiers
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.Equal(t, audio.Codec, out.Codec)
	require.Equal(t, audio.Tiers[0].TargetAverageBitrate, out.Tiers[0].TargetAverageBitrate)
}

func TestCameraCapabilitiesRoundTrip(t *testing.T) {
	uuid := SensorUUIDBytes("test-camera")
	require.Len(t, uuid, 16)

	caps := DefaultCameraCapabilities(uuid)
	b, err := tlv8.Marshal(caps)
	require.NoError(t, err)

	var out CameraCapabilitiesValue
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.Equal(t, uint8(1), out.Version)
	require.Len(t, out.CameraSensors.Sensors, 1)
	require.Equal(t, uuid, out.CameraSensors.Sensors[0].SensorUUID)
	require.Equal(t, byte(SensorTypePrimary), out.CameraSensors.Sensors[0].SensorType)
	require.Len(t, out.CameraSensors.Sensors[0].VideoStreamCapabilities, 3)
}

func TestWebRTCSolicitOfferTLV8(t *testing.T) {
	req := WebRTCSolicitOfferRequest{
		Options: WebRTCOfferOptions{SFrameEnabled: true},
	}
	b, err := tlv8.Marshal(req)
	require.NoError(t, err)

	var outReq WebRTCSolicitOfferRequest
	require.NoError(t, tlv8.Unmarshal(b, &outReq))
	require.True(t, outReq.Options.SFrameEnabled)

	res := WebRTCSolicitOfferResponse{
		SessionIdentifier: string(make([]byte, 16)),
		SDPOffer:          "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n",
		Status:            WebRTCSolicitSuccess,
		SFrameConfiguration: SFrameKeyData{
			Key: string(make([]byte, 16)),
			KID: 1,
		},
	}
	b, err = tlv8.Marshal(res)
	require.NoError(t, err)

	var outRes WebRTCSolicitOfferResponse
	require.NoError(t, tlv8.Unmarshal(b, &outRes))
	require.Equal(t, res.SDPOffer, outRes.SDPOffer)
	require.Equal(t, byte(WebRTCSolicitSuccess), outRes.Status)
	require.Equal(t, uint64(1), outRes.SFrameConfiguration.KID)
}

func TestWebRTCProvideAnswerTLV8(t *testing.T) {
	req := WebRTCProvideAnswerRequest{
		SessionIdentifier: "0123456789abcdef",
		SDPAnswer:         "v=0\r\n",
		AdditionalCandidates: []WebRTCICECandidate{
			{Candidate: "candidate:1 1 udp 1 1.2.3.4 1234 typ host", SDPMid: "0", SDPMLineIndex: 0},
		},
	}
	b, err := tlv8.Marshal(req)
	require.NoError(t, err)

	var out WebRTCProvideAnswerRequest
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.Equal(t, req.SessionIdentifier, out.SessionIdentifier)
	require.Equal(t, req.SDPAnswer, out.SDPAnswer)
	require.Len(t, out.AdditionalCandidates, 1)
	require.Equal(t, "0", out.AdditionalCandidates[0].SDPMid)
}

func TestCSRAndCertTLV8(t *testing.T) {
	req := CameraClientCSRRequest{Nonce: string(make([]byte, 32))}
	b, err := tlv8.Marshal(req)
	require.NoError(t, err)
	var outReq CameraClientCSRRequest
	require.NoError(t, tlv8.Unmarshal(b, &outReq))
	require.Len(t, outReq.Nonce, 32)

	res := CameraClientCSRResponse{CSR: "csr-der", NonceSignature: "sig"}
	b, err = tlv8.Marshal(res)
	require.NoError(t, err)
	var outRes CameraClientCSRResponse
	require.NoError(t, tlv8.Unmarshal(b, &outRes))
	require.Equal(t, "csr-der", outRes.CSR)

	status := CameraClientCertificateStatusValue{NeedsUpdate: true}
	b, err = tlv8.Marshal(status)
	require.NoError(t, err)
	var outStatus CameraClientCertificateStatusValue
	require.NoError(t, tlv8.Unmarshal(b, &outStatus))
	require.True(t, outStatus.NeedsUpdate)
}

func TestNewHKSVAccessory(t *testing.T) {
	acc := NewHKSVAccessory("AlexxIT", "go2rtc", "cam", "SN", "1.0", "seed1")
	require.NotNil(t, acc)
	require.Equal(t, uint8(hap.DeviceAID), acc.AID)

	// Accessories information + 5 RTP + mic + 10 HKSV services
	require.GreaterOrEqual(t, len(acc.Services), 1+MinConcurrentRTPSessions+1+10)

	// All IIDs unique and non-zero
	seen := map[uint64]bool{}
	for _, srv := range acc.Services {
		require.NotZero(t, srv.IID)
		require.False(t, seen[srv.IID], "duplicate service IID %x", srv.IID)
		seen[srv.IID] = true
		for _, ch := range srv.Characters {
			require.NotZero(t, ch.IID)
			require.False(t, seen[ch.IID], "duplicate char IID %x type=%s", ch.IID, ch.Type)
			seen[ch.IID] = true
		}
	}

	// Required HKSV services present
	require.NotNil(t, acc.GetService(TypeCameraCapabilities))
	require.NotNil(t, acc.GetService(TypeCameraWebRTCStreamManagement))
	require.NotNil(t, acc.GetService(TypeCameraMultiTierRTPStreamManagement))
	require.NotNil(t, acc.GetService(TypeCameraBufferManagement))
	require.NotNil(t, acc.GetService(TypeCameraKeyManagement))
	require.NotNil(t, acc.GetService(TypeCameraClientCertificateManagement))
	require.NotNil(t, acc.GetService(TypeCameraGlobalOperatingMode))

	// Capabilities version 17.99
	capSvc := acc.GetService(TypeCameraCapabilities)
	ver := capSvc.GetCharacter(TypeVersion)
	require.NotNil(t, ver)
	require.Equal(t, CameraCapabilitiesVersion, ver.Value)

	// WebRTC service has solicit-offer with wr permission
	webrtcSvc := acc.GetService(TypeCameraWebRTCStreamManagement)
	solicit := webrtcSvc.GetCharacter(TypeWebRTCSolicitOffer)
	require.NotNil(t, solicit)
	require.Contains(t, solicit.Perms, "wr")

	// Multi-tier advertises AES_CM_128_HMAC_SHA1_80 only
	multi := acc.GetService(TypeCameraMultiTierRTPStreamManagement)
	rtpChar := multi.GetCharacter(TypeSupportedRTPConfiguration)
	require.NotNil(t, rtpChar)
	var rtp SupportedRTPConfiguration
	require.NoError(t, rtpChar.ReadTLV8(&rtp))
	require.Equal(t, []byte{CryptoAES_CM_128_HMAC_SHA1_80}, rtp.SRTPCryptoType)

	// Video tiers include HEVC
	vidChar := multi.GetCharacter(TypeSupportedVideoStreamTiers)
	require.NotNil(t, vidChar)
	s, ok := vidChar.Value.(string)
	require.True(t, ok)
	raw, err := base64.StdEncoding.DecodeString(s)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
}

func TestLegacyAccessoryStillWorks(t *testing.T) {
	acc := NewAccessory("AlexxIT", "go2rtc", "cam", "SN", "1.0")
	require.NotNil(t, acc)
	require.NotNil(t, acc.GetService("110"))
	// No HKSV services by default
	require.Nil(t, acc.GetService(TypeCameraWebRTCStreamManagement))
}

func TestBoolTLV8(t *testing.T) {
	type wrap struct {
		Flag bool `tlv8:"1"`
	}
	b, err := tlv8.Marshal(wrap{Flag: true})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1}, b)

	var out wrap
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.True(t, out.Flag)

	b, err = tlv8.Marshal(wrap{Flag: false})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 0}, b)
}

func TestBufferAndKeyTLV8(t *testing.T) {
	pub := CameraRecordingPublishingPointValue{
		URL: "https://example.com/cmaf/",
		ServerCACertificates: []Certificate{
			{Certificate: "der-cert"},
		},
	}
	b, err := tlv8.Marshal(pub)
	require.NoError(t, err)
	var out CameraRecordingPublishingPointValue
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.Equal(t, "https://example.com/cmaf/", out.URL)
	require.Len(t, out.ServerCACertificates, 1)

	key := CameraKeyValue{Key: "secret", KeyNumber: 42}
	b, err = tlv8.Marshal(key)
	require.NoError(t, err)
	var outKey CameraKeyValue
	require.NoError(t, tlv8.Unmarshal(b, &outKey))
	require.Equal(t, uint64(42), outKey.KeyNumber)
}

func TestRTPStreamingControlTLV8(t *testing.T) {
	req := RTPStreamingControlRequest{
		SessionIdentifier: "sess",
		Command:           RTPStreamCommandStart,
		VideoTier:         1,
		VideoSSRC:         123,
		AudioTier:         1,
		AudioSSRC:         456,
	}
	b, err := tlv8.Marshal(req)
	require.NoError(t, err)
	var out RTPStreamingControlRequest
	require.NoError(t, tlv8.Unmarshal(b, &out))
	require.Equal(t, uint32(1), out.VideoTier)
	require.Equal(t, uint32(123), out.VideoSSRC)
	require.Equal(t, byte(RTPStreamCommandStart), out.Command)
}
