package camera

const TypeSetupEndpoints = "118"

type SetupEndpointsRequest struct {
	SessionID   string          `tlv8:"1"`
	Address     Address         `tlv8:"3"`
	VideoCrypto SRTPCryptoSuite `tlv8:"4"`
	AudioCrypto SRTPCryptoSuite `tlv8:"5"`
}

type SetupEndpointsResponse struct {
	SessionID   string          `tlv8:"1"`
	Status      byte            `tlv8:"2"`
	Address     Address         `tlv8:"3"`
	VideoCrypto SRTPCryptoSuite `tlv8:"4"`
	AudioCrypto SRTPCryptoSuite `tlv8:"5"`
	VideoSSRC   uint32          `tlv8:"6"`
	AudioSSRC   uint32          `tlv8:"7"`
}

type Address struct {
	IPVersion    byte   `tlv8:"1"`
	IPAddr       string `tlv8:"2"`
	VideoRTPPort uint16 `tlv8:"3"`
	AudioRTPPort uint16 `tlv8:"4"`
}

type SRTPCryptoSuite struct {
	CryptoSuite byte   `tlv8:"1"`
	MasterKey   string `tlv8:"2"` // 16 (AES_CM_128) or 32 (AES_256_CM)
	MasterSalt  string `tlv8:"3"` // 14 byte
}
