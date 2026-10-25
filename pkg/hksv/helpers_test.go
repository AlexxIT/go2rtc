// Author: Sergei "svk" Krashevich <svk@svk.su>
package hksv

import (
	"crypto/ed25519"
	"encoding/hex"
	"regexp"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/stretchr/testify/require"
)

// --- CalcName ---

func TestCalcName_CustomName(t *testing.T) {
	require.Equal(t, "MyCamera", CalcName("MyCamera", "anything"))
}

func TestCalcName_Generated(t *testing.T) {
	name := CalcName("", "camera1")
	require.Regexp(t, `^go2rtc-[0-9A-F]{4}$`, name)
}

func TestCalcName_Deterministic(t *testing.T) {
	require.Equal(t, CalcName("", "seed"), CalcName("", "seed"))
}

func TestCalcName_DifferentSeeds(t *testing.T) {
	require.NotEqual(t, CalcName("", "a"), CalcName("", "b"))
}

// --- CalcDeviceID ---

var macRe = regexp.MustCompile(`^[0-9A-F]{2}(:[0-9A-F]{2}){5}$`)

func TestCalcDeviceID_Generated(t *testing.T) {
	id := CalcDeviceID("", "seed")
	require.Regexp(t, macRe, id)
}

func TestCalcDeviceID_CustomFull(t *testing.T) {
	// Full MAC-length ID returned as-is
	require.Equal(t, "AA:BB:CC:DD:EE:FF", CalcDeviceID("AA:BB:CC:DD:EE:FF", "seed"))
}

func TestCalcDeviceID_CustomShort(t *testing.T) {
	// Short custom ID used as seed instead
	id := CalcDeviceID("short", "seed")
	require.Regexp(t, macRe, id)
	// Should differ from empty seed because "short" is used as seed
	require.NotEqual(t, CalcDeviceID("", "seed"), id)
}

func TestCalcDeviceID_Deterministic(t *testing.T) {
	require.Equal(t, CalcDeviceID("", "cam1"), CalcDeviceID("", "cam1"))
}

// --- CalcDevicePrivate ---

func TestCalcDevicePrivate_Generated(t *testing.T) {
	key := CalcDevicePrivate("", "seed")
	require.Len(t, key, ed25519.PrivateKeySize)
}

func TestCalcDevicePrivate_ValidHex(t *testing.T) {
	// Generate a key, encode to hex, pass back — should get same key
	original := CalcDevicePrivate("", "seed")
	hexStr := hex.EncodeToString(original)
	restored := CalcDevicePrivate(hexStr, "other-seed")
	require.Equal(t, original, restored)
}

func TestCalcDevicePrivate_InvalidHex(t *testing.T) {
	// Invalid hex treated as seed
	key := CalcDevicePrivate("not-hex", "seed")
	require.Len(t, key, ed25519.PrivateKeySize)
	// "not-hex" is used as seed, not "seed"
	require.NotEqual(t, CalcDevicePrivate("", "seed"), key)
}

func TestCalcDevicePrivate_ShortHex(t *testing.T) {
	// Valid hex but too short for ed25519 — treated as seed
	key := CalcDevicePrivate("abcd", "seed")
	require.Len(t, key, ed25519.PrivateKeySize)
}

func TestCalcDevicePrivate_Deterministic(t *testing.T) {
	require.Equal(t, CalcDevicePrivate("", "x"), CalcDevicePrivate("", "x"))
}

func TestCalcDevicePrivate_SignsCorrectly(t *testing.T) {
	// Verify the generated key is actually usable for signing
	key := ed25519.PrivateKey(CalcDevicePrivate("", "seed"))
	msg := []byte("test message")
	sig := ed25519.Sign(key, msg)
	pub := key.Public().(ed25519.PublicKey)
	require.True(t, ed25519.Verify(pub, msg, sig))
}

// --- CalcSetupID ---

func TestCalcSetupID(t *testing.T) {
	id := CalcSetupID("seed")
	require.Regexp(t, `^[0-9A-F]{4}$`, id)
}

func TestCalcSetupID_Deterministic(t *testing.T) {
	require.Equal(t, CalcSetupID("x"), CalcSetupID("x"))
}

func TestCalcSetupID_DifferentSeeds(t *testing.T) {
	require.NotEqual(t, CalcSetupID("a"), CalcSetupID("b"))
}

// --- CalcCategoryID ---

func TestCalcCategoryID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", hap.CategoryCamera},
		{"camera", hap.CategoryCamera},
		{"bridge", hap.CategoryBridge},
		{"doorbell", hap.CategoryDoorbell},
		{"5", "5"},
		{"17", "17"},
		{"0", hap.CategoryCamera},   // Atoi("0") == 0, not > 0
		{"abc", hap.CategoryCamera}, // unknown string
	}
	for _, tc := range tests {
		t.Run("input="+tc.input, func(t *testing.T) {
			require.Equal(t, tc.expected, CalcCategoryID(tc.input))
		})
	}
}
