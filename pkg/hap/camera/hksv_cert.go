package camera

// CameraClientCSRRequest is written by the controller with a nonce (UUID 8081)
type CameraClientCSRRequest struct {
	Nonce string `tlv8:"1"` // 32 random bytes
}

// CameraClientCSRResponse is the CSR + nonce signature from the accessory
type CameraClientCSRResponse struct {
	CSR            string `tlv8:"1"` // DER CSR
	NonceSignature string `tlv8:"2"` // EC signature of nonce, max 128 bytes
}

// CameraClientCertificateRequest installs the issued client cert (UUID 8082)
type CameraClientCertificateRequest struct {
	ClientCertificate string `tlv8:"1"` // DER
	CA                string `tlv8:"2"` // DER
}

// CameraClientCertificateStatusValue reports cert freshness (UUID 8083)
type CameraClientCertificateStatusValue struct {
	NeedsUpdate bool `tlv8:"1"`
}
