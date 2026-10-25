package camera

import (
	"encoding/base64"
	"encoding/hex"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/google/uuid"
)

func dataBase64(raw string) string {
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// SensorUUIDBytes returns a stable 16-byte sensor UUID string for TLV8 data fields
func SensorUUIDBytes(seed string) string {
	u := uuid.NewSHA1(uuid.NameSpaceOID, []byte("go2rtc/hksv/"+seed))
	return string(u[:])
}

// SensorUUIDHex returns the sensor UUID as a hex string (for logs/config)
func SensorUUIDHex(seed string) string {
	u := uuid.NewSHA1(uuid.NameSpaceOID, []byte("go2rtc/hksv/"+seed))
	return hex.EncodeToString(u[:])
}

func mustTLV8(v any) string {
	s, err := tlv8.MarshalBase64(v)
	if err != nil {
		return ""
	}
	return s
}

// ServiceCameraCapabilities advertises open-source HKSV camera capabilities (17.99)
func ServiceCameraCapabilities(sensorUUID string) *hap.Service {
	return &hap.Service{
		Type: TypeCameraCapabilities,
		Characters: []*hap.Character{
			{
				Type:   TypeVersion,
				Format: hap.FormatString,
				Value:  CameraCapabilitiesVersion,
				Perms:  hap.PR,
			},
			{
				Type:   TypeCameraCapabilitiesChar,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(DefaultCameraCapabilities(sensorUUID)),
				Perms:  hap.PR,
			},
		},
	}
}

// ServiceCameraGlobalOperatingMode controls whole-accessory streaming state
func ServiceCameraGlobalOperatingMode() *hap.Service {
	return &hap.Service{
		Type: TypeCameraGlobalOperatingMode,
		Characters: []*hap.Character{
			{
				Type:   TypeHomeKitCameraActive,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   TypeStreamingEnabled,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVTWPRPW,
			},
			{
				Type:   TypeCameraOperatingModeIndicator,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPRPW,
			},
		},
	}
}

// ServiceMotionSensorHKSV is a motion sensor with optional multi-sensor fields
func ServiceMotionSensorHKSV(sensorUUID string) *hap.Service {
	return &hap.Service{
		Type: TypeMotionSensor,
		Characters: []*hap.Character{
			{
				Type:   TypeMotionDetected,
				Format: hap.FormatBool,
				Value:  false,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeStatusActive,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeMotionEnabled,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVTWPRPW,
			},
			{
				Type:   TypeSensorUUID,
				Format: hap.FormatData,
				Value:  dataBase64(sensorUUID),
				Perms:  hap.PR,
			},
		},
	}
}

// ServiceCameraMotionZones holds activity zones
func ServiceCameraMotionZones() *hap.Service {
	return &hap.Service{
		Type: TypeCameraMotionZones,
		Characters: []*hap.Character{
			{
				Type:   TypeVersion,
				Format: hap.FormatString,
				Value:  CameraCapabilitiesVersion,
				Perms:  hap.PR,
			},
			{
				Type:   TypeActive,
				Format: hap.FormatUInt8,
				Value:  1,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   TypeCameraZones,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(CameraZonesValue{ZoneDataVersion: 2}),
				Perms:  hap.PRPW,
			},
		},
	}
}

// ServiceCameraMultiTierRTPStreamManagement advertises multi-tier RTP streaming
func ServiceCameraMultiTierRTPStreamManagement(sensorUUID string) *hap.Service {
	videoH264 := Default1080pVideoTiers(VideoCodecTypeTierH264, 99)
	videoH265 := Default1080pVideoTiers(VideoCodecTypeTierH265, 100)
	// Combined list: marshal as separate codec entries by using first as primary
	// and advertising H.265 via a second SupportedVideoStreamTiers value is not
	// possible as a single TLV8; HomeKit expects one value that may list one codec.
	// We advertise H.265 as primary (mandatory) and keep H.264 as secondary service
	// instance if needed. Spec: Codec enum on the characteristic is singular, so
	// accessories typically expose one characteristic value per codec via repeated
	// top-level items. Our tlv8 slice marshal supports that via []SupportedVideoStreamTiers.
	_ = videoH264

	audio := DefaultOpusAudioTier(111)
	rtp := SupportedRTPConfiguration{
		SRTPCryptoType: []byte{CryptoAES_CM_128_HMAC_SHA1_80},
	}

	return &hap.Service{
		Type: TypeCameraMultiTierRTPStreamManagement,
		Characters: []*hap.Character{
			{
				Type:   TypeStreamingEnabled,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVTWPRPW,
			},
			{
				Type:   TypeStatusActive,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeSupportedVideoStreamTiers,
				Format: hap.FormatTLV8,
				// Prefer HEVC (mandatory) as the primary advertised codec
				Value: mustTLV8([]SupportedVideoStreamTiers{videoH265, videoH264}),
				Perms: hap.EVPR,
			},
			{
				Type:   TypeSupportedAudioStreamTiers,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(audio),
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeSupportedRTPConfiguration,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(rtp),
				Perms:  hap.PR,
			},
			{
				Type:   TypeSetupEndpoints,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPW,
			},
			{
				Type:   TypeRTPStreamingControl,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeSensorUUID,
				Format: hap.FormatData,
				Value:  dataBase64(sensorUUID),
				Perms:  hap.PR,
			},
		},
	}
}

// ServiceCameraWebRTCStreamManagement is the HAP WebRTC signalling service
func ServiceCameraWebRTCStreamManagement(sensorUUID string) *hap.Service {
	videoH265 := Default1080pVideoTiers(VideoCodecTypeTierH265, 100)
	videoH264 := Default1080pVideoTiers(VideoCodecTypeTierH264, 99)
	audio := DefaultOpusAudioTier(111)

	return &hap.Service{
		Type: TypeCameraWebRTCStreamManagement,
		Characters: []*hap.Character{
			{
				Type:   TypeWebRTCSolicitOffer,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeWebRTCProvideAnswer,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeWebRTCStreamingControl,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeWebRTCNumberOfActiveSessions,
				Format: hap.FormatUInt8,
				Value:  0,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeWebRTCReoffer,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeWebRTCUpdateSession,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeWebRTCSupportedVideoStreamTiers,
				Format: hap.FormatTLV8,
				Value:  mustTLV8([]SupportedVideoStreamTiers{videoH265, videoH264}),
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeWebRTCSupportedAudioStreamTiers,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(audio),
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeStreamingEnabled,
				Format: hap.FormatBool,
				Value:  true,
				Perms:  hap.EVTWPRPW,
			},
			{
				Type:   TypeSensorUUID,
				Format: hap.FormatData,
				Value:  dataBase64(sensorUUID),
				Perms:  hap.PR,
			},
		},
	}
}

// ServiceCameraRecordingManagementHKSV enables event recording control
func ServiceCameraRecordingManagementHKSV() *hap.Service {
	return &hap.Service{
		Type: TypeCameraRecordingManagement,
		Characters: []*hap.Character{
			{
				Type:   TypeActive,
				Format: hap.FormatUInt8,
				Value:  0,
				Perms:  hap.EVPRPW,
			},
			{
				Type:   TypeRecordingAudioActive,
				Format: hap.FormatUInt8,
				Value:  0,
				Perms:  hap.EVPRPW,
			},
		},
	}
}

// ServiceCameraBufferManagement manages recording buffers and CMAF publish point
func ServiceCameraBufferManagement() *hap.Service {
	return &hap.Service{
		Type: TypeCameraBufferManagement,
		Characters: []*hap.Character{
			{
				Type:   TypeBufferUploadCommand,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeBufferActivityCommand,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PW,
			},
			{
				Type:   TypeBufferEventCommand,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeBufferEventSequenceNumber,
				Format: hap.FormatUInt32,
				Value:  0,
				Perms:  hap.EVPR,
			},
			{
				Type:   TypeCameraRecordingPublishingPoint,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPW,
			},
		},
	}
}

// ServiceCameraKeyManagement holds CMAF ingest keys
func ServiceCameraKeyManagement() *hap.Service {
	return &hap.Service{
		Type: TypeCameraKeyManagement,
		Characters: []*hap.Character{
			{
				Type:   TypeCameraKey,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  []string{"pw", "tw"},
			},
			{
				Type:   TypeCameraKeyID,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(CameraKeyIDValue{KeyID: 0}),
				Perms:  hap.EVPR,
			},
		},
	}
}

// ServiceCameraClientCertificateManagement handles CMAF client cert provisioning
func ServiceCameraClientCertificateManagement() *hap.Service {
	return &hap.Service{
		Type: TypeCameraClientCertificateManagement,
		Characters: []*hap.Character{
			{
				Type:   TypeCameraClientCSR,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  hap.PRPWWR,
			},
			{
				Type:   TypeCameraClientCertificate,
				Format: hap.FormatTLV8,
				Value:  "",
				Perms:  []string{"pr", "pw", "tw"},
			},
			{
				Type:   TypeCameraClientCertificateStatus,
				Format: hap.FormatTLV8,
				Value:  mustTLV8(CameraClientCertificateStatusValue{NeedsUpdate: true}),
				Perms:  hap.EVPR,
			},
		},
	}
}
