package camera

const TypeSupportedRTPConfiguration = "116"

//goland:noinspection ALL
const (
	CryptoAES_CM_128_HMAC_SHA1_80 = 0
	CryptoAES_CM_256_HMAC_SHA1_80 = 1
	CryptoDisabled                = 2
)

type SupportedRTPConfiguration struct {
	SRTPCryptoType []byte `tlv8:"2"`
}
