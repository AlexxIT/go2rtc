package camera

// HAP short type UUIDs from HomeKit Secure Video Open Source Compatibility Guide
// Full form: 0000XXXX-0000-1000-8000-0026BB765291
// Developer Preview 17.99 (2026-06-03)

// Service types
const (
	TypeCameraCapabilities                 = "8010"
	TypeCameraGlobalOperatingMode          = "8032"
	TypeCameraMotionZones                  = "8021"
	TypeCameraBufferManagement             = "8000"
	TypeCameraMultiTierRTPStreamManagement = "8031"
	TypeCameraWebRTCStreamManagement       = "8033"
	// TypeCameraRecordingManagement already exists as "204" in HAP R2
	TypeCameraKeyManagement                = "8050"
	TypeCameraClientCertificateManagement  = "8080"
	TypeMotionSensor                       = "85"
)

// Characteristic types
const (
	TypeSensorUUID                         = "805B"
	TypeMotionEnabled                      = "8087"
	TypeSupportedVideoStreamTiers          = "8043"
	TypeSupportedAudioStreamTiers          = "8044"
	TypeCameraCapabilitiesChar             = "8011"
	TypeContributingSensors                = "8086"
	TypeCameraKey                          = "8051"
	TypeCameraKeyID                        = "8052"
	TypeBufferUploadCommand                = "8013"
	TypeBufferActivityCommand              = "8017"
	TypeBufferEventCommand                 = "8014"
	TypeBufferEventSequenceNumber          = "8015"
	TypeCameraRecordingPublishingPoint     = "8016"
	TypeCameraZones                        = "8022"
	TypeStreamingEnabled                   = "8041"
	TypeRTPStreamingControl                = "8045"
	TypeWebRTCSolicitOffer                 = "8053"
	TypeWebRTCProvideAnswer                = "8054"
	TypeWebRTCStreamingControl             = "8056"
	TypeWebRTCNumberOfActiveSessions       = "8057"
	TypeWebRTCReoffer                      = "8058"
	TypeWebRTCUpdateSession                = "805C"
	TypeWebRTCSupportedVideoStreamTiers    = "8059"
	TypeWebRTCSupportedAudioStreamTiers    = "805A"
	TypeCameraClientCSR                    = "8081"
	TypeCameraClientCertificate            = "8082"
	TypeCameraClientCertificateStatus      = "8083"

	// Existing HAP characteristics/services reused by HKSV services
	TypeVersion                      = "37"
	TypeActive                       = "B0"
	TypeCameraOperatingMode          = "21A" // legacy operating mode service
	TypeHomeKitCameraActive          = "21B"
	TypeThirdPartyCameraActive       = "21C"
	TypeCameraOperatingModeIndicator = "21D"
	TypeMotionDetected               = "22"
	TypeRecordingAudioActive         = "226"
	TypeNightVision                  = "11B"
	TypeStatusActive                 = "75"
	TypeCameraRecordingManagement    = "204"
)

// CameraCapabilitiesVersion is the Developer Preview version string
const CameraCapabilitiesVersion = "17.99"

// Video codec type for stream tiers (differs from legacy VideoCodecTypeH264=0)
const (
	VideoCodecTypeTierH264 = 1
	VideoCodecTypeTierH265 = 2
)

// Camera video quality tiers
const (
	VideoQualityHighest = 1
	VideoQualityHigh    = 2
	VideoQualityMedium  = 3
	VideoQualityLow     = 4
)

// Audio sample rates for stream tiers
const (
	AudioTierSampleRate16kHz = 1
	AudioTierSampleRate24kHz = 2
	AudioTierSampleRate32kHz = 3
	AudioTierSampleRate48kHz = 4
)

// Audio bit depth for stream tiers
const (
	AudioTierBitDepth8  = 1
	AudioTierBitDepth16 = 2
	AudioTierBitDepth24 = 3
)

// Sensor type / intent
const (
	SensorTypeUnknown = 0
	SensorTypePrimary = 1
	SensorTypeGeneric = 255

	SensorIntentUnknown = 0
	SensorIntentMain    = 1
	SensorIntentPackage = 2
	SensorIntentGeneric = 255
)

// RTP streaming control commands
const (
	RTPStreamCommandEnd   = 1
	RTPStreamCommandStart = 2
)

// RTP / WebRTC streaming status
const (
	StreamStatusSuccess                  = 0
	StreamStatusUnknownSessionIdentifier = 1
	StreamStatusNoSuchStream             = 2
	StreamStatusBusy                     = 3
	StreamStatusError                    = 4
)

// WebRTC solicit-offer status
const (
	WebRTCSolicitSuccess           = 0
	WebRTCSolicitPrivacyModeActive = 1
	WebRTCSolicitError             = 2
)

// WebRTC streaming status (provide answer / control / reoffer / update)
const (
	WebRTCStatusSuccess                  = 0
	WebRTCStatusUnknownSessionIdentifier = 1
	WebRTCStatusBusy                     = 2
	WebRTCStatusError                    = 3
)

// WebRTC streaming control commands
const (
	WebRTCCommandEnd = 1
)

// Buffer upload commands
const (
	BufferUploadStart         = 1
	BufferUploadStartAndStop  = 2
	BufferUploadStop          = 3
	BufferStopActionPause     = 1
	BufferStopActionFinalize  = 2
)

// Buffer activity
const (
	BufferActivityShouldRecord    = 1
	BufferActivityShouldNotRecord = 2
)

// Buffer event commands / types
const (
	BufferEventQuery       = 1
	BufferEventAcknowledge = 2

	BufferEventTypeCMAFSessionStart = 1
	BufferEventTypeCMAFSessionStop  = 2
	BufferEventTypeMotion           = 3
	BufferEventTypeCMAFError        = 4
)

// Zone application methods
const (
	ZoneMethodNormal   = 1
	ZoneMethodInverted = 2
)

// Default bitrates (kbps) from the guide
const (
	Bitrate4KAvgKbps    = 4500
	Bitrate4KMaxKbps    = 5000
	Bitrate2KAvgKbps    = 2800
	Bitrate2KMaxKbps    = 3000
	Bitrate1080pAvgKbps = 1700
	Bitrate1080pMaxKbps = 1800
	Bitrate720pAvgKbps  = 768
	Bitrate720pMaxKbps  = 800
	BitrateLowAvgKbps   = 180
	BitrateLowMaxKbps   = 190
)

// Max concurrent sessions required by the guide
const (
	MinConcurrentRTPSessions    = 5
	MinConcurrentWebRTCSessions = 6
)
