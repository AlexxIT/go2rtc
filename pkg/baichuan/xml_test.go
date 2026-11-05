package baichuan

import (
	"strings"
	"testing"
)

func TestParseNonceLimits(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(strings.Repeat(" ", maxNonceBody+1)),
		[]byte(`<body><Encryption><nonce>` + strings.Repeat("n", maxNonceSize+1) +
			`</nonce></Encryption></body>`),
	} {
		if _, err := parseNonce(body); err == nil {
			t.Fatal("accepted oversized login nonce")
		}
	}
}

func FuzzXMLParsers(f *testing.F) {
	f.Add([]byte(`<body><Encryption><nonce>nonce</nonce></Encryption></body>`))
	f.Add([]byte(`<Extension><binaryData>1</binaryData><encryptLen>8</encryptLen></Extension>`))
	f.Add([]byte(`<body><TalkAbility version="1.1"><duplexList><duplex>fullDuplex</duplex></duplexList></TalkAbility></body>`))
	f.Add([]byte(`<body><AbilityInfo><system><subModule><abilityValue>version_ro</abilityValue></subModule></system></AbilityInfo></body>`))
	f.Fuzz(func(t *testing.T, body []byte) {
		_, _ = parseNonce(body)
		_, _ = parseExtension(body)
		_, _ = decodeTalkAbility(body)
		_, _ = parseCapabilities(body)
		_, _ = parseDeviceInfo(body)
	})
}
