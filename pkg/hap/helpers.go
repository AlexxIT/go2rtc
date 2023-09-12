package hap

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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

	StatusPaired    = "0"
	StatusNotPaired = "1"

	CategoryBridge   = "2"
	CategoryCamera   = "17"
	CategoryDoorbell = "18"

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

	PermissionUser  = 0
	PermissionAdmin = 1
)

const DeviceAID = 1 // TODO: fix someday

type JSONAccessories struct {
	Value []*Accessory `json:"accessories"`
}

type JSONCharacters struct {
	Value []JSONCharacter `json:"characteristics"`
}

type JSONCharacter struct {
	AID   uint8  `json:"aid"`
	IID   uint64 `json:"iid"`
	Value any    `json:"value,omitempty"`
	Event any    `json:"ev,omitempty"`
}

func SanitizePin(pin string) (string, error) {
	s := strings.ReplaceAll(pin, "-", "")
	if len(s) != 8 {
		return "", errors.New("hap: wrong PIN format: " + pin)
	}
	// 123-45-678
	return s[:3] + "-" + s[3:5] + "-" + s[5:], nil
}

func GenerateKey() []byte {
	_, key, _ := ed25519.GenerateKey(nil)
	return key
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

func newRequestError(req any) error {
	return fmt.Errorf("hap: wrong request: %#v", req)
}

func newResponseError(req, res any) error {
	return fmt.Errorf("hap: wrong response: %#v, on request: %#v", res, req)
}
