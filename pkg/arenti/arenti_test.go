package arenti

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
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

func TestFormatAPIError(t *testing.T) {
	if err := FormatAPIError("1001", "OK"); err != nil {
		t.Fatalf("expected nil for code 1001, got %v", err)
	}

	if err := FormatAPIError("1017", ""); err == nil || !strings.Contains(err.Error(), "incorrect password") {
		t.Fatalf("expected incorrect password error for 1017, got %v", err)
	}

	if err := FormatAPIError("1018", ""); err == nil || !strings.Contains(err.Error(), "account does not exist") {
		t.Fatalf("expected account does not exist error for 1018, got %v", err)
	}

	if err := FormatAPIError("1023", ""); err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("expected authentication failed error for 1023, got %v", err)
	}

	if err := FormatAPIError("9999", ""); err == nil || !strings.Contains(err.Error(), "unknown server error (code 9999)") {
		t.Fatalf("expected unknown server error for 9999, got %v", err)
	}

	// Verify checkError clears UserToken on 1023
	client := NewClient("user@example.com", "pass", "EU")
	client.UserToken = "active-token"
	_ = client.checkError("1023", "")
	if client.UserToken != "" {
		t.Fatalf("expected UserToken to be cleared on 1023, got %q", client.UserToken)
	}
}

func TestRetryOnCode1023(t *testing.T) {
	var loginCalls int32
	var listCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ipc_web/user/login":
			atomic.AddInt32(&loginCalls, 1)
			_, _ = w.Write([]byte(`{"code":"1001","msg":"OK","data":{"userID":12345,"userToken":"fresh-token-456"}}`))
		case "/ipc_web/device/list":
			call := atomic.AddInt32(&listCalls, 1)
			if call == 1 {
				// Simulate session invalidated by another login
				_, _ = w.Write([]byte(`{"code":"1023","msg":"session invalidated"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":"1001","msg":"OK","data":{"ipc":[{"deviceID":999,"deviceName":"Cam1"}]}}`))
		}
	}))
	defer srv.Close()

	client := NewClient("user@example.com", "SecretPass123", "EU")
	client.SetBaseURL(srv.URL)
	client.UserID = 12345
	client.UserToken = "stale-token-000"

	devices, err := client.GetDevices()
	if err != nil {
		t.Fatalf("expected GetDevices to succeed after 1023 retry, got error: %v", err)
	}
	if len(devices) != 1 || devices[0].DeviceName != "Cam1" {
		t.Fatalf("unexpected devices returned: %+v", devices)
	}
	if atomic.LoadInt32(&loginCalls) != 1 {
		t.Fatalf("expected 1 re-login call, got %d", loginCalls)
	}
	if atomic.LoadInt32(&listCalls) != 2 {
		t.Fatalf("expected 2 device list calls, got %d", listCalls)
	}
	if client.UserToken != "fresh-token-456" {
		t.Fatalf("expected updated token 'fresh-token-456', got %q", client.UserToken)
	}
}

func TestParseChannelStream(t *testing.T) {
	tests := []struct {
		url     string
		channel int
		stream  int
	}{
		{"arenti://camera1", 1, 0},
		{"arenti://camera1?stream=0", 1, 0},
		{"arenti://camera1?stream=hd", 1, 0},
		{"arenti://camera1?stream=main", 1, 0},
		{"arenti://camera1?stream=1", 1, 1},
		{"arenti://camera1?stream=sd", 1, 1},
		{"arenti://camera1?stream=sub", 1, 1},
		{"arenti://camera1?subtype=1", 1, 1},
		{"arenti://camera1?channel=2", 2, 0},
		{"arenti://camera1?channel=3&stream=sd", 3, 1},
		{"arenti://camera1?stream=sd#comment", 1, 1},
	}

	for _, tt := range tests {
		ch, st := ParseChannelStream(tt.url)
		if ch != tt.channel || st != tt.stream {
			t.Errorf("ParseChannelStream(%q) = (%d, %d), expected (%d, %d)", tt.url, ch, st, tt.channel, tt.stream)
		}
	}
}
