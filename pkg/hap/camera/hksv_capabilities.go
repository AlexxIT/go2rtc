package camera

// CameraCapabilitiesValue is the Camera Capabilities characteristic (UUID 8011)
type CameraCapabilitiesValue struct {
	Version        uint8          `tlv8:"1"`
	CameraSensors  CameraSensors  `tlv8:"2"`
}

// CameraSensors wraps a list of sensor configurations
type CameraSensors struct {
	Sensors []SensorConfiguration `tlv8:"1"`
}

// SensorConfiguration describes one image sensor
type SensorConfiguration struct {
	SensorDimensions      SensorDimensions               `tlv8:"1"`
	SensorUUID            string                         `tlv8:"2"` // 16-byte UUID
	SensorType            byte                           `tlv8:"3"`
	SensorIntent          byte                           `tlv8:"4"`
	VideoStreamCapabilities []CameraVideoStreamCapability `tlv8:"5"`
}

// SensorDimensions is the sensor pixel size
type SensorDimensions struct {
	Width  uint16 `tlv8:"1"`
	Height uint16 `tlv8:"2"`
}

// CameraVideoStreamCapability is one advertised stream configuration
type CameraVideoStreamCapability struct {
	Identifier        string `tlv8:"1"` // UUID
	VideoQuality      byte   `tlv8:"2"`
	Width             uint16 `tlv8:"3"`
	Height            uint16 `tlv8:"4"`
	FramesPerSecond   uint8  `tlv8:"5"`
	AverageBitRate    uint32 `tlv8:"6"` // kbps
	PeakBitRate       uint32 `tlv8:"7"` // kbps
}

// ContributingSensorsValue lists sensors that contributed to motion (UUID 8086)
type ContributingSensorsValue struct {
	SensorList []ContributingSensor `tlv8:"1"`
}

// ContributingSensor is one sensor UUID in a motion event
type ContributingSensor struct {
	SensorUUID string `tlv8:"1"`
}

// CameraZonesValue holds motion/activity zones (UUID 8022)
type CameraZonesValue struct {
	ZoneDataVersion uint8  `tlv8:"1"`
	ZoneData        string `tlv8:"2"` // opaque Zone Data TLV8 blob
}

// ZoneDataV2 is version-2 zone data content
type ZoneDataV2 struct {
	Method   byte      `tlv8:"1"`
	Polygons []Polygon `tlv8:"3"`
}

// Polygon is a non-self-intersecting zone polygon
type Polygon struct {
	Identifier string `tlv8:"1"` // UUID
	Vertices   string `tlv8:"3"` // little-endian UINT16 (X,Y) pairs
}

// DefaultCameraCapabilities builds a 1080p primary sensor capabilities blob
func DefaultCameraCapabilities(sensorUUID string) CameraCapabilitiesValue {
	return CameraCapabilitiesValue{
		Version: 1,
		CameraSensors: CameraSensors{
			Sensors: []SensorConfiguration{
				{
					SensorDimensions: SensorDimensions{Width: 1920, Height: 1080},
					SensorUUID:       sensorUUID,
					SensorType:       SensorTypePrimary,
					SensorIntent:     SensorIntentMain,
					VideoStreamCapabilities: []CameraVideoStreamCapability{
						{
							Identifier:      sensorUUID,
							VideoQuality:    VideoQualityHigh,
							Width:           1920,
							Height:          1080,
							FramesPerSecond: 30,
							AverageBitRate:  Bitrate1080pAvgKbps,
							PeakBitRate:     Bitrate1080pMaxKbps,
						},
						{
							Identifier:      sensorUUID,
							VideoQuality:    VideoQualityMedium,
							Width:           1280,
							Height:          720,
							FramesPerSecond: 30,
							AverageBitRate:  Bitrate720pAvgKbps,
							PeakBitRate:     Bitrate720pMaxKbps,
						},
						{
							Identifier:      sensorUUID,
							VideoQuality:    VideoQualityLow,
							Width:           640,
							Height:          360,
							FramesPerSecond: 15,
							AverageBitRate:  BitrateLowAvgKbps,
							PeakBitRate:     BitrateLowMaxKbps,
						},
					},
				},
			},
		},
	}
}
