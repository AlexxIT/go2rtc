package camera

const TypeSupportedRTPConfiguration = "116"

//goland:noinspection ALL
const (
	CryptoAES_CM_128_HMAC_SHA1_80 = 0
	CryptoAES_CM_256_HMAC_SHA1_80 = 1
	CryptoNone                    = 2
)

type SupportedRTPConfig struct {
	CryptoType []byte `tlv8:"2"`
}
