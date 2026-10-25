package tuya

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const testLocalKey = "0123456789abcdef"

func TestTuyaLanV34FrameRoundTrip(t *testing.T) {
	key := []byte(testLocalKey)
	payload := []byte(`{"type":"offer","sdp":"v=0"}`)

	raw, err := encodeV34Frame(7, 3, payload, key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, v34Prefix) || !bytes.HasSuffix(raw, v34Suffix) {
		t.Fatal("frame is not wrapped in the 3.4 prefix/suffix")
	}
	if got := binary.BigEndian.Uint32(raw[4:8]); got != 7 {
		t.Fatalf("sequence = %d, want 7", got)
	}
	if got := binary.BigEndian.Uint32(raw[8:12]); got != 3 {
		t.Fatalf("command = %d, want 3", got)
	}
	if got, want := binary.BigEndian.Uint32(raw[12:16]), uint32(len(raw)-16); got != want {
		t.Fatalf("length = %d, want %d", got, want)
	}

	frame, err := readV34Frame(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame, raw) {
		t.Fatal("readV34Frame did not return the encoded frame")
	}

	command, _, decoded, err := decodeV34Frame(frame, key, false)
	if err != nil {
		t.Fatal(err)
	}
	if command != 3 || !bytes.Equal(decoded, payload) {
		t.Fatalf("command = %d payload = %q, want 3 %q", command, decoded, payload)
	}

	// A payload that is already a multiple of the AES block size must still round-trip.
	aligned := []byte("0123456789abcdef")
	raw, err = encodeV34Frame(0, 1, aligned, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, decoded, err = decodeV34Frame(raw, key, false); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, aligned) {
		t.Fatalf("payload = %q, want %q", decoded, aligned)
	}
}

func TestTuyaLanV34FrameRejectsTampering(t *testing.T) {
	key := []byte(testLocalKey)
	raw, err := encodeV34Frame(1, 4, []byte("hello"), key)
	if err != nil {
		t.Fatal(err)
	}

	flipped := append([]byte(nil), raw...)
	flipped[20] ^= 0xff
	if _, _, _, err = decodeV34Frame(flipped, key, false); err == nil {
		t.Fatal("payload modification was not detected")
	}
	if _, _, _, err = decodeV34Frame(raw, []byte("fedcba9876543210"), false); err == nil {
		t.Fatal("wrong local key was not detected")
	}
	if _, _, _, err = decodeV34Frame(raw[:len(raw)-1], key, false); err == nil {
		t.Fatal("truncated frame was not detected")
	}
}

func TestTuyaLanReadFrameRejectsBadHeader(t *testing.T) {
	header := make([]byte, 16)
	copy(header, v34Prefix)
	binary.BigEndian.PutUint32(header[12:16], 4)
	if _, err := readV34Frame(bytes.NewReader(header)); err == nil {
		t.Fatal("too small frame length was accepted")
	}

	header = make([]byte, 16)
	copy(header, []byte{1, 2, 3, 4})
	binary.BigEndian.PutUint32(header[12:16], 64)
	if _, err := readV34Frame(bytes.NewReader(header)); err == nil {
		t.Fatal("invalid frame prefix was accepted")
	}
}

func TestTuyaLanAnswerNormalizedForHEVC(t *testing.T) {
	// HEVC cameras announce their video as a Tuya application section, which Pion cannot
	// negotiate. Only the G.711 audio m-line may survive.
	answer := strings.Join([]string{
		"v=0",
		"a=group:BUNDLE 0 1 2",
		"m=audio 9 UDP/TLS/RTP/SAVPF 0",
		"a=mid:0",
		"a=sendrecv",
		"a=rtpmap:0 PCMU/8000",
		"m=video 9 UDP/TLS/RTP/SAVPF 45",
		"a=mid:1",
		"m=application 9 tuya",
		"a=mid:2",
	}, "\r\n") + "\r\n"

	got := normalizeTuyaLanAnswer(answer)
	if !strings.Contains(got, "m=audio 9 UDP/TLS/RTP/SAVPF 0") {
		t.Fatalf("audio section was dropped:\n%s", got)
	}
	if strings.Contains(got, "m=application 9 tuya") {
		t.Fatalf("Tuya application section survived:\n%s", got)
	}
	if !strings.Contains(got, "a=group:BUNDLE 0 1\r\n") {
		t.Fatalf("bundle group was not reduced:\n%s", got)
	}
	if !strings.Contains(got, "m=video 0 UDP/TLS/RTP/SAVPF 102") ||
		!strings.Contains(got, "a=inactive") {
		t.Fatalf("video section was not replaced with an inactive placeholder:\n%s", got)
	}

	// Answers without the Tuya video section must pass through untouched.
	plain := "v=0\r\nm=audio 9 UDP/TLS/RTP/SAVPF 0\r\n"
	if normalizeTuyaLanAnswer(plain) != plain {
		t.Fatal("plain answer was modified")
	}
}

func TestNewTuyaLanApiClientFromURL(t *testing.T) {
	src := "tuya-lan://192.168.1.10?device_id=abc123&local_key=" + testLocalKey +
		"&skill=%7B%22audios%22%3A%5B%7B%22channels%22%3A1%2C%22codecType%22%3A101%2C%22sampleRate%22%3A8000%7D%5D%7D"
	u, err := url.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewTuyaLanApiClient(u)
	if err != nil {
		t.Fatal(err)
	}
	if client.lan.config.CameraIP != "192.168.1.10" || client.lan.config.DeviceID != "abc123" {
		t.Fatalf("camera_ip = %q device_id = %q", client.lan.config.CameraIP, client.lan.config.DeviceID)
	}
	if client.lan.config.Port != 6668 || client.lan.config.HGWVersion != "3.4" {
		t.Fatalf("port = %d hgw_version = %q", client.lan.config.Port, client.lan.config.HGWVersion)
	}
	if client.lan.config.SenderID == "" {
		t.Fatal("sender_id was not generated")
	}
	if len(client.skill.Audios) != 1 {
		t.Fatalf("skill audios = %d, want 1", len(client.skill.Audios))
	}

	// The local key must be validated before any connection is attempted.
	u, _ = url.Parse("tuya-lan://192.168.1.10?device_id=abc123&local_key=short")
	if _, err = NewTuyaLanApiClient(u); err == nil {
		t.Fatal("short local_key was accepted")
	}
	u, _ = url.Parse("tuya-lan://192.168.1.10?local_key=" + testLocalKey)
	if _, err = NewTuyaLanApiClient(u); err == nil {
		t.Fatal("missing device_id was accepted")
	}
	u, _ = url.Parse("tuya-lan://192.168.1.10?device_id=abc123&local_key=" + testLocalKey + "&hgw_version=3.5")
	if _, err = NewTuyaLanApiClient(u); err == nil {
		t.Fatal("unsupported hgw_version was accepted")
	}
	u, _ = url.Parse("tuya-lan:?device_id=abc123")
	if _, err = NewTuyaLanApiClient(u); err == nil {
		t.Fatal("source without camera address or config path was accepted")
	}
}

func TestNewTuyaLanApiClientFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tuya-lan.json")
	body, err := json.Marshal(TuyaLanConfig{
		CameraIP: "10.0.0.5",
		DeviceID: "device-1",
		SenderID: "sender-1",
		LocalKey: testLocalKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}

	u, _ := url.Parse("tuya-lan:" + path)
	client, err := NewTuyaLanApiClient(u)
	if err != nil {
		t.Fatal(err)
	}
	if client.lan.config.CameraIP != "10.0.0.5" || client.lan.config.SenderID != "sender-1" {
		t.Fatalf("camera_ip = %q sender_id = %q", client.lan.config.CameraIP, client.lan.config.SenderID)
	}
	if client.lan.config.Port != 6668 || client.lan.config.HGWVersion != "3.4" {
		t.Fatalf("port = %d hgw_version = %q", client.lan.config.Port, client.lan.config.HGWVersion)
	}

	u, _ = url.Parse("tuya-lan:" + filepath.Join(t.TempDir(), "missing.json"))
	if _, err = NewTuyaLanApiClient(u); err == nil {
		t.Fatal("missing config file was accepted")
	}
}

func TestTuyaLanAudioCodecs(t *testing.T) {
	client := &TuyaLanAPIClient{}
	if codecs := client.GetAudioCodecs(); len(codecs) != 2 ||
		codecs[0].Name != core.CodecPCMA || codecs[1].Name != core.CodecPCMU {
		t.Fatalf("default codecs = %v, want PCMA+PCMU", codecs)
	}

	// Tuya reports the camera's G.711 audio as codecType 101 (PCML in the cloud dialect).
	// Over the LAN the answer negotiates G.711, so both flavours must be advertised for a
	// browser microphone to match the backchannel track.
	client.skill = &Skill{Audios: []AudioSkill{{CodecType: 101, SampleRate: 8000, Channels: 1}}}
	codecs := client.GetAudioCodecs()
	if len(codecs) != 2 {
		t.Fatalf("codecs = %v, want 2 entries", codecs)
	}
	names := []string{codecs[0].Name, codecs[1].Name}
	if !(names[0] == core.CodecPCMA && names[1] == core.CodecPCMU) &&
		!(names[0] == core.CodecPCMU && names[1] == core.CodecPCMA) {
		t.Fatalf("codecs = %v, want the G.711 pair", names)
	}
	for _, codec := range codecs {
		if codec.Name == core.CodecPCML {
			t.Fatalf("codecs = %v, LAN must not advertise PCML", names)
		}
		if codec.ClockRate != 8000 || codec.Channels != 1 {
			t.Fatalf("codec = %v, want 8000/1", codec)
		}
	}
}

func TestLanRetryDelay(t *testing.T) {
	// Invariants: the first failure waits the minimum, every further failure waits longer,
	// and the wait is capped so a dead camera cannot stall the stream forever.
	if got := lanRetryDelay(1); got != lanRetryMin {
		t.Fatalf("first failure: got %s, want %s", got, lanRetryMin)
	}

	prev := time.Duration(0)
	for failures := 1; failures <= 12; failures++ {
		got := lanRetryDelay(failures)
		if got < prev {
			t.Fatalf("delay decreased at failure %d: %s < %s", failures, got, prev)
		}
		if got > lanRetryMax {
			t.Fatalf("delay exceeded cap at failure %d: %s > %s", failures, got, lanRetryMax)
		}
		prev = got
	}

	if got := lanRetryDelay(64); got != lanRetryMax {
		t.Fatalf("cap: got %s, want %s", got, lanRetryMax)
	}
}

func TestLanRetryAllow(t *testing.T) {
	// Contract: a failed attempt closes the window (no camera traffic until it expires), a
	// successful session clears the state, and both are observable through Allow/Remaining.
	retry := new(lanRetryState)
	if !retry.Allow() {
		t.Fatal("fresh state must allow a session")
	}
	if d := retry.Remaining(); d != 0 {
		t.Fatalf("fresh state remaining = %s, want 0", d)
	}

	retry.Failed()
	if retry.Allow() {
		t.Fatal("must not allow a session right after a failure")
	}
	if d := retry.Remaining(); d <= 0 || d > lanRetryMax {
		t.Fatalf("remaining = %s, want (0, %s]", d, lanRetryMax)
	}

	retry.OK()
	if !retry.Allow() {
		t.Fatal("a successful session must clear the backoff")
	}
}

func TestLanRetryIsPerCamera(t *testing.T) {
	var retries lanRetryRegistry
	first := retries.For("camera-1")
	second := retries.For("camera-2")

	first.Failed()
	if first.Allow() {
		t.Fatal("failed camera must enter backoff")
	}
	if !second.Allow() {
		t.Fatal("failure of camera-1 must not suppress camera-2")
	}
	if got := retries.For("camera-1"); got != first {
		t.Fatal("same camera must retain its retry history across dials")
	}
}
