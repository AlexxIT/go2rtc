// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/hex"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/hap"
)

// CalcName generates a HomeKit display name from a seed if name is empty.
func CalcName(name, seed string) string {
	if name != "" {
		return name
	}
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("go2rtc-%02X%02X", b[0], b[2])
}

// CalcDeviceID generates a MAC-like device ID from a seed if deviceID is empty.
func CalcDeviceID(deviceID, seed string) string {
	if deviceID != "" {
		if len(deviceID) >= 17 {
			return deviceID
		}
		seed = deviceID
	}
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X", b[32], b[34], b[36], b[38], b[40], b[42])
}

// CalcDevicePrivate generates an ed25519 private key from a seed if private is empty.
func CalcDevicePrivate(private, seed string) []byte {
	if private != "" {
		if b, _ := hex.DecodeString(private); len(b) == ed25519.PrivateKeySize {
			return b
		}
		seed = private
	}
	b := sha512.Sum512([]byte(seed))
	return ed25519.NewKeyFromSeed(b[:ed25519.SeedSize])
}

// CalcSetupID generates a setup ID from a seed.
func CalcSetupID(seed string) string {
	b := sha512.Sum512([]byte(seed))
	return fmt.Sprintf("%02X%02X", b[44], b[46])
}

// CalcCategoryID converts a category string to a HAP category constant.
func CalcCategoryID(categoryID string) string {
	switch categoryID {
	case "bridge":
		return hap.CategoryBridge
	case "doorbell":
		return hap.CategoryDoorbell
	}
	if core.Atoi(categoryID) > 0 {
		return categoryID
	}
	return hap.CategoryCamera
}
