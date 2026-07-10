package camera

import (
	"github.com/AlexxIT/go2rtc/pkg/hap"
)

// NewHKSVAccessory builds a HomeKit camera accessory with the Developer Preview
// HKSV open-source services (WebRTC, multi-tier HEVC, CMAF provisioning)
// in addition to classic RTP stream management (min 5 concurrent RTP sessions)
func NewHKSVAccessory(manuf, model, name, serial, firmware, seed string) *hap.Accessory {
	sensorUUID := SensorUUIDBytes(seed)

	services := []*hap.Service{
		hap.ServiceAccessoryInformation(manuf, model, name, serial, firmware),
	}
	// Classic live view slots (guide requires at least 5 concurrent RTP sessions)
	for i := 0; i < MinConcurrentRTPSessions; i++ {
		services = append(services, ServiceCameraRTPStreamManagement())
	}
	services = append(services,
		ServiceMicrophone(),
		// HKSV open-source services (spec 17.99)
		ServiceCameraCapabilities(sensorUUID),
		ServiceCameraGlobalOperatingMode(),
		ServiceMotionSensorHKSV(sensorUUID),
		ServiceCameraMotionZones(),
		ServiceCameraMultiTierRTPStreamManagement(sensorUUID),
		ServiceCameraWebRTCStreamManagement(sensorUUID),
		ServiceCameraRecordingManagementHKSV(),
		ServiceCameraBufferManagement(),
		ServiceCameraKeyManagement(),
		ServiceCameraClientCertificateManagement(),
	)

	acc := &hap.Accessory{
		AID:      hap.DeviceAID,
		Services: services,
	}
	acc.InitIID()
	return acc
}

// AppendHKSVServices adds HKSV services to an existing accessory (in-place)
// Call InitIID after if IIDs have not been assigned yet
func AppendHKSVServices(acc *hap.Accessory, seed string) {
	sensorUUID := SensorUUIDBytes(seed)
	acc.Services = append(acc.Services,
		ServiceCameraCapabilities(sensorUUID),
		ServiceCameraGlobalOperatingMode(),
		ServiceMotionSensorHKSV(sensorUUID),
		ServiceCameraMotionZones(),
		ServiceCameraMultiTierRTPStreamManagement(sensorUUID),
		ServiceCameraWebRTCStreamManagement(sensorUUID),
		ServiceCameraRecordingManagementHKSV(),
		ServiceCameraBufferManagement(),
		ServiceCameraKeyManagement(),
		ServiceCameraClientCertificateManagement(),
	)
}
