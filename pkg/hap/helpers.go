package hap

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	TXTConfigNumber = "c#" // Current configuration number (ex. 1, 2, 3)
	TXTDeviceID     = "id" // Device ID of the accessory (ex. 77:75:87:A0:7D:F4)
	TXTModel        = "md" // Model name of the accessory (ex. MJCTD02YL)
	TXTProtoVersion = "pv" // Protocol version string (ex. 1.1)
	TXTStateNumber  = "s#" // Current state number (ex. 1)
	TXTCategory     = "ci" // Accessory Category Identifier (ex. 2, 5, 17)
	TXTSetupHash    = "sh" // Setup hash (ex. Y9w9hQ==)

	// TXTFeatureFlags
	//  - 0001b - Supports Apple Authentication Coprocessor
	//  - 0010b - Supports Software Authentication
	TXTFeatureFlags = "ff" // Pairing Feature flags (ex. 0, 1, 2)

	// TXTStatusFlags
	//  - 0001b - Accessory has not been paired with any controllers
	//  - 0100b - A problem has been detected on the accessory
	TXTStatusFlags = "sf" // Status flags (ex. 0, 1)

	StateM1 = 1
	StateM2 = 2
	StateM3 = 3
	StateM4 = 4
	StateM5 = 5
	StateM6 = 6

	MethodPair          = 0
	MethodPairMFi       = 1 // if device has MFI cert
	MethodVerifyPair    = 2
	MethodAddPairing    = 3
	MethodDeletePairing = 4
	MethodListPairings  = 5
)

const (
	PermissionUser  = 0
	PermissionAdmin = 1
)

const DeviceAID = 1 // TODO: fix someday

func GenerateKey() []byte {
	_, key, _ := ed25519.GenerateKey(nil)
	return key
}

func GenerateID(name string) string {
	sum := sha512.Sum512([]byte(name))
	return fmt.Sprintf(
		"%02X:%02X:%02X:%02X:%02X:%02X",
		sum[0], sum[1], sum[2], sum[3], sum[4], sum[5],
	)
}

func GenerateUUID() string {
	//12345678-9012-3456-7890-123456789012
	data := make([]byte, 16)
	_, _ = rand.Read(data)
	s := hex.EncodeToString(data)
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

func Append(items ...any) (b []byte) {
	for _, item := range items {
		switch v := item.(type) {
		case string:
			b = append(b, v...)
		case []byte:
			b = append(b, v[:]...)
		default:
			panic(v)
		}
	}
	return
}

func NewResponseError(req, res any) error {
	return fmt.Errorf("hap: wrong response: %#v, on request: %#v", res, req)
}

func UnmarshalEvent(res *http.Response) (char *Character, err error) {
	var data []byte
	if data, err = io.ReadAll(res.Body); err != nil {
		return
	}

	ch := Characters{}
	if err = json.Unmarshal(data, &ch); err != nil {
		return
	}

	if len(ch.Characters) > 1 {
		panic("not implemented")
	}

	char = ch.Characters[0]
	return
}
