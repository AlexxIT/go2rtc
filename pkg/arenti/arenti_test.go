package arenti

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestEncryptPassword(t *testing.T) {
	enc, err := EncryptPassword("SecretPassword123!")
	if err != nil {
		t.Fatalf("EncryptPassword failed: %v", err)
	}

	// Encrypted double-base64 string should be non-empty and decodable
	raw1, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("first base64 decode failed: %v", err)
	}

	raw2, err := base64.StdEncoding.DecodeString(string(raw1))
	if err != nil {
		t.Fatalf("second base64 decode failed: %v", err)
	}

	// 1024-bit RSA ciphertext is 128 bytes
	if len(raw2) != 128 {
		t.Fatalf("expected 128 bytes RSA ciphertext, got %d", len(raw2))
	}
}

func TestCalcSign(t *testing.T) {
	// Test HMAC calculation with known vectors
	identity := "100001273858"
	timestamp := "1788510000000"
	nonce := "0123456789abcdef0123456789abcdef"
	params := "deviceid=aa2d2bc5598e4e4c"
	body := ""
	key := "test-secret-key"

	sign := CalcSign(identity, timestamp, nonce, params, body, key)
	if len(sign) != 64 {
		t.Fatalf("expected 64 hex chars signature, got %d (%s)", len(sign), sign)
	}

	// Must be uppercase hex
	if sign != strings.ToUpper(sign) {
		t.Fatalf("signature must be uppercase hex: %s", sign)
	}
}

func TestNewUUID(t *testing.T) {
	uuid := NewUUID()
	matched, err := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`, uuid)
	if err != nil || !matched {
		t.Fatalf("invalid UUID v4 generated: %s", uuid)
	}
}

func TestFormatOfferSDP(t *testing.T) {
	rawSDP := "v=0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 96\r\n" +
		"a=rtpmap:96 H264/90000\r\n" +
		"a=extmap:1 urn:ietf:params:rtp-hdrext:sdes:mid\r\n" +
		"a=fmtp:96 level-asymmetry-allowed=1\r\n" +
		"m=audio 9 UDP/TLS/RTP/SAVPF 0\r\n" +
		"a=rtpmap:0 PCMU/8000\r\n" +
		"a=extmap:2 urn:ietf:params:rtp-hdrext:ssrc-audio-level\r\n"

	filtered := formatOfferSDP(rawSDP)

	// Video section must not have extmap
	if strings.Contains(filtered, "a=extmap:1") {
		t.Fatalf("expected a=extmap:1 to be stripped from video: %s", filtered)
	}

	// Audio section keeps extmap or whatever outside video
	if !strings.Contains(filtered, "a=extmap:2") {
		t.Fatalf("expected audio extmap to be retained: %s", filtered)
	}
}

func TestDefaultCountry(t *testing.T) {
	client := NewClient("user@example.com", "SecretPassword123!", "")
	if client.CountryCode != "US" {
		t.Fatalf("expected default country code to be US, got %s", client.CountryCode)
	}
	if client.BaseURL != DefaultBaseURLUS {
		t.Fatalf("expected default BaseURL to be US (%s), got %s", DefaultBaseURLUS, client.BaseURL)
	}
}

func TestResolveBaseURL(t *testing.T) {
	// Country-based
	if url := ResolveBaseURL("US", "", ""); url != DefaultBaseURLUS {
		t.Fatalf("expected US to resolve to %s, got %s", DefaultBaseURLUS, url)
	}
	if url := ResolveBaseURL("CA", "", ""); url != DefaultBaseURLUS {
		t.Fatalf("expected CA to resolve to %s, got %s", DefaultBaseURLUS, url)
	}
	if url := ResolveBaseURL("DK", "", ""); url != DefaultBaseURLEU {
		t.Fatalf("expected DK to resolve to %s, got %s", DefaultBaseURLEU, url)
	}
	if url := ResolveBaseURL("DE", "", ""); url != DefaultBaseURLEU {
		t.Fatalf("expected DE to resolve to %s, got %s", DefaultBaseURLEU, url)
	}

	// Region override
	if url := ResolveBaseURL("DK", "us", ""); url != DefaultBaseURLUS {
		t.Fatalf("expected region 'us' to override to %s, got %s", DefaultBaseURLUS, url)
	}
	if url := ResolveBaseURL("US", "eu", ""); url != DefaultBaseURLEU {
		t.Fatalf("expected region 'eu' to override to %s, got %s", DefaultBaseURLEU, url)
	}

	// Server override
	if url := ResolveBaseURL("US", "eu", "https://custom.arenti.net/"); url != "https://custom.arenti.net" {
		t.Fatalf("expected server override, got %s", url)
	}
}
