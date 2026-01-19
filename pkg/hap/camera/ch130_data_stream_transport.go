package camera

const TypeSupportedDataStreamTransportConfiguration = "130"

type SupportedDataStreamTransportConfiguration struct {
	Configs []TransferTransportConfiguration `tlv8:"1"`
}

type TransferTransportConfiguration struct {
	TransportType byte `tlv8:"1"`
}
