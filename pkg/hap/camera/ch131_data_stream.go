package camera

const TypeSetupDataStreamTransport = "131"

type SetupDataStreamTransportRequest struct {
	SessionCommandType byte   `tlv8:"1"`
	TransportType      byte   `tlv8:"2"`
	ControllerKeySalt  string `tlv8:"3"`
}

type SetupDataStreamTransportResponse struct {
	Status                         byte `tlv8:"1"`
	TransportTypeSessionParameters struct {
		TCPListeningPort uint16 `tlv8:"1"`
	} `tlv8:"2"`
	AccessoryKeySalt string `tlv8:"3"`
}
