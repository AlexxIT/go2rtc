package homekit

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/camera"
	"github.com/AlexxIT/go2rtc/pkg/hap/chacha20poly1305"
	"github.com/AlexxIT/go2rtc/pkg/hap/curve25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/ed25519"
	"github.com/AlexxIT/go2rtc/pkg/hap/hkdf"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/AlexxIT/go2rtc/pkg/yaml"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func testCamera(t *testing.T) *server {
	t.Helper()
	oldHosts, oldServers, oldLog, oldPath := hosts, servers, log, app.ConfigPath
	log = zerolog.Nop()
	app.ConfigPath = filepath.Join(t.TempDir(), "go2rtc.yaml")
	require.NoError(t, os.WriteFile(app.ConfigPath, []byte("homekit:\n  porch:\n    name: Porch\n"), 0600))
	s := &server{
		stream:    "porch",
		mdns:      &mdns.ServiceEntry{Name: "Porch", Info: map[string]string{hap.TXTStatusFlags: hap.StatusNotPaired}},
		accessory: camera.NewAccessory("test", "test", "Porch", "test", "test"),
	}
	s.hap = &hap.Server{
		Pin: "482-61-937", DeviceID: "11:22:33:44:55:66",
		DevicePrivate: hap.GenerateKey(), GetClientPublic: s.GetPair,
	}
	hosts = map[string]*server{"porch.local": s}
	servers = map[string]*server{"porch": s}
	t.Cleanup(func() {
		hosts, servers, log, app.ConfigPath = oldHosts, oldServers, oldLog, oldPath
	})
	return s
}

func getStatus(t *testing.T, client *http.Client, method, url string, want int) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(t, err)
	require.Equal(t, want, res.StatusCode, url)
}

func TestDedicatedHAPListenerIsolation(t *testing.T) {
	s := testCamera(t)
	oldMux, oldPort := http.DefaultServeMux, api.Port
	http.DefaultServeMux, api.Port = http.NewServeMux(), 0 // API disabled
	t.Cleanup(func() { http.DefaultServeMux, api.Port = oldMux, oldPort })
	api.HandleFunc("api/homekit", apiHomekit)

	other := &mdns.ServiceEntry{}
	srv, err := serveHAP("127.0.0.1:0", []*mdns.ServiceEntry{s.mdns, other})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	_, port, err := net.SplitHostPort(srv.Addr)
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(int(s.mdns.Port)), port)
	require.NotZero(t, s.mdns.Port)
	require.Equal(t, s.mdns.Port, other.Port)

	client := &http.Client{Timeout: 3 * time.Second}
	for _, path := range []string{"/", "/api/config", "/api/streams", "/api/frame.jpeg", "/api/homekit", "/api/homekit/accessories", "/api/discovery/homekit", "/accessories", "/characteristics", "/resource"} {
		getStatus(t, client, "GET", "http://"+srv.Addr+path, http.StatusNotFound)
	}
	getStatus(t, client, "GET", "http://"+srv.Addr+hap.PathPairSetup, http.StatusMethodNotAllowed)
	getStatus(t, client, "GET", "http://"+srv.Addr+hap.PathPairVerify, http.StatusMethodNotAllowed)

	apiServer := httptest.NewServer(http.DefaultServeMux)
	defer apiServer.Close()
	getStatus(t, client, "GET", apiServer.URL+"/api/homekit", http.StatusOK)
	getStatus(t, client, "POST", apiServer.URL+hap.PathPairSetup, http.StatusNotFound)
	getStatus(t, client, "POST", apiServer.URL+hap.PathPairVerify, http.StatusNotFound)
}

func TestHAPListenerBindFailureDoesNotFallBack(t *testing.T) {
	oldMux := http.DefaultServeMux
	http.DefaultServeMux = http.NewServeMux()
	t.Cleanup(func() { http.DefaultServeMux = oldMux })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	entry := &mdns.ServiceEntry{Port: 12345}
	srv, err := serveHAP(ln.Addr().String(), []*mdns.ServiceEntry{entry})
	require.Error(t, err)
	require.Nil(t, srv)
	require.Equal(t, uint16(12345), entry.Port)
	for _, path := range []string{hap.PathPairSetup, hap.PathPairVerify} {
		req := httptest.NewRequest("POST", path, nil)
		_, pattern := http.DefaultServeMux.Handler(req)
		require.Empty(t, pattern)
	}
}

func TestLegacyHAPListener(t *testing.T) {
	oldMux, oldPort := http.DefaultServeMux, api.Port
	http.DefaultServeMux, api.Port = http.NewServeMux(), 1984
	t.Cleanup(func() { http.DefaultServeMux, api.Port = oldMux, oldPort })
	entry := &mdns.ServiceEntry{}
	srv, err := serveHAP("", []*mdns.ServiceEntry{entry})
	require.NoError(t, err)
	require.Nil(t, srv)
	require.Equal(t, uint16(1984), entry.Port)
	for _, path := range []string{hap.PathPairSetup, hap.PathPairVerify} {
		req := httptest.NewRequest("POST", path, nil)
		_, pattern := http.DefaultServeMux.Handler(req)
		require.Equal(t, path, pattern)
	}
}

func TestHAPPairAndVerify(t *testing.T) {
	s := testCamera(t)
	srv, err := serveHAP("127.0.0.1:0", []*mdns.ServiceEntry{s.mdns})
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	client := &hap.Client{DeviceAddress: srv.Addr, DeviceID: s.hap.DeviceID, ClientID: hap.GenerateUUID(), ClientPrivate: hap.GenerateKey()}
	require.NoError(t, client.Pair("", s.hap.Pin))
	// PairSetup writes its response before persisting the pairing. Waiting for
	// EOF also ensures the hijacked handler has finished before reading state.
	require.NoError(t, client.Conn.SetReadDeadline(time.Now().Add(3*time.Second)))
	_, err = io.ReadAll(client.Conn)
	require.NoError(t, err)
	require.NoError(t, client.Close())
	require.Equal(t, client.ClientPublic(), s.GetPair(client.ClientID))
	data, err := os.ReadFile(app.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(data), client.ClientID)

	for i := 0; i < 2; i++ {
		verifySession(t, srv.Addr, client, s, true)
	}
	// Recreate the accessory from the saved pairing and move its listener.
	// The controller must be able to verify again without repeating PairSetup.
	var stored struct {
		Homekit map[string]struct {
			Pairings []string `yaml:"pairings"`
		} `yaml:"homekit"`
	}
	require.NoError(t, yaml.Unmarshal(data, &stored))
	restored := &server{
		stream: s.stream, pairings: stored.Homekit[s.stream].Pairings,
		mdns:      &mdns.ServiceEntry{Name: "Porch", Info: map[string]string{}},
		accessory: s.accessory,
	}
	restored.hap = &hap.Server{Pin: s.hap.Pin, DeviceID: s.hap.DeviceID, DevicePrivate: s.hap.DevicePrivate, GetClientPublic: restored.GetPair}
	require.NoError(t, srv.Close())
	s = restored
	hosts = map[string]*server{s.mdns.Host(mdns.ServiceHAP): s}
	servers = map[string]*server{s.stream: s}
	srv, err = serveHAP("127.0.0.1:0", []*mdns.ServiceEntry{s.mdns})
	require.NoError(t, err)
	verifySession(t, srv.Addr, client, s, true)

	unknown := &hap.Client{DeviceID: client.DeviceID, DevicePublic: client.DevicePublic, ClientID: hap.GenerateUUID(), ClientPrivate: hap.GenerateKey()}
	verifySession(t, srv.Addr, unknown, s, false)
	unknown.ClientID = client.ClientID // known ID with an incorrect signing key
	verifySession(t, srv.Addr, unknown, s, false)

	// Two accessories must remain distinguishable on the same listener.
	other := &server{
		stream: "indoor", pairings: append([]string(nil), s.pairings...),
		mdns:      &mdns.ServiceEntry{Name: "Indoor", Info: map[string]string{}},
		accessory: camera.NewAccessory("test", "test", "Indoor", "other", "test"),
	}
	other.hap = &hap.Server{DeviceID: "22:33:44:55:66:77", DevicePrivate: hap.GenerateKey(), GetClientPublic: other.GetPair}
	hosts = map[string]*server{s.mdns.Host(mdns.ServiceHAP): s, other.mdns.Host(mdns.ServiceHAP): other}
	verifySession(t, srv.Addr, client, s, true)
	indoorClient := &hap.Client{DeviceID: other.hap.DeviceID, DevicePublic: other.hap.ServerPublic(), ClientID: client.ClientID, ClientPrivate: client.ClientPrivate}
	verifySession(t, srv.Addr, indoorClient, other, true)
}

// Exercise PairVerify without mDNS discovery or any non-loopback traffic.
func openVerifiedSession(t *testing.T, address string, client *hap.Client, s *server, authorized bool) *hap.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	require.NoError(t, err)
	return verifyConnection(t, conn, address, client, s, authorized)
}

func verifyConnection(t *testing.T, conn net.Conn, address string, client *hap.Client, s *server, authorized bool) *hap.Conn {
	t.Helper()
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetDeadline(time.Now().Add(3*time.Second)))
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	post := func(value any, result any) error {
		req, err := http.NewRequest("POST", "http://"+address+hap.PathPairVerify, tlv8.MarshalReader(value))
		require.NoError(t, err)
		req.Host = s.mdns.Host(mdns.ServiceHAP)
		req.Header.Set("Content-Type", hap.MimeTLV8)
		require.NoError(t, req.Write(conn))
		res, err := http.ReadResponse(rw.Reader, req)
		if err != nil {
			return err
		}
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.NoError(t, tlv8.UnmarshalReader(res.Body, res.ContentLength, result))
		require.NoError(t, res.Body.Close())
		return nil
	}
	public, private := curve25519.GenerateKeyPair()
	m1 := struct {
		State     byte   `tlv8:"6"`
		PublicKey string `tlv8:"3"`
	}{hap.StateM1, string(public)}
	var m2 struct {
		State         byte   `tlv8:"6"`
		PublicKey     string `tlv8:"3"`
		EncryptedData string `tlv8:"5"`
	}
	require.NoError(t, post(m1, &m2))
	require.Equal(t, byte(hap.StateM2), m2.State)
	shared, err := curve25519.SharedSecret(private, []byte(m2.PublicKey))
	require.NoError(t, err)
	key, err := hkdf.Sha512(shared, "Pair-Verify-Encrypt-Salt", "Pair-Verify-Encrypt-Info")
	require.NoError(t, err)
	decoded, err := chacha20poly1305.Decrypt(key, "PV-Msg02", []byte(m2.EncryptedData))
	require.NoError(t, err)
	var accessory struct {
		Identifier string `tlv8:"1"`
		Signature  string `tlv8:"10"`
	}
	require.NoError(t, tlv8.Unmarshal(decoded, &accessory))
	require.Equal(t, client.DeviceID, accessory.Identifier)
	require.True(t, ed25519.ValidateSignature(client.DevicePublic, hap.Append(m2.PublicKey, accessory.Identifier, public), []byte(accessory.Signature)))
	signature, err := ed25519.Signature(client.ClientPrivate, hap.Append(public, client.ClientID, m2.PublicKey))
	require.NoError(t, err)
	plain, err := tlv8.Marshal(struct {
		Identifier string `tlv8:"1"`
		Signature  string `tlv8:"10"`
	}{client.ClientID, string(signature)})
	require.NoError(t, err)
	encrypted, err := chacha20poly1305.Encrypt(key, "PV-Msg03", plain)
	require.NoError(t, err)
	var m4 struct {
		State byte `tlv8:"6"`
	}
	err = post(struct {
		State         byte   `tlv8:"6"`
		EncryptedData string `tlv8:"5"`
	}{hap.StateM3, string(encrypted)}, &m4)
	if !authorized {
		require.Error(t, err)
		return nil
	}
	require.NoError(t, err)
	require.Equal(t, byte(hap.StateM4), m4.State)
	secure, err := hap.NewConn(conn, rw, shared, true)
	require.NoError(t, err)
	return secure
}

func verifySession(t *testing.T, address string, client *hap.Client, s *server, authorized bool, checks ...func(*hap.Conn)) {
	t.Helper()
	secure := openVerifiedSession(t, address, client, s, authorized)
	if secure == nil {
		return
	}
	if len(checks) == 0 {
		req, err := http.NewRequest("GET", "http://"+address+hap.PathAccessories, nil)
		require.NoError(t, err)
		require.NoError(t, req.Write(secure))
		res, err := http.ReadResponse(bufio.NewReader(secure), req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		expected, err := json.Marshal(hap.JSONAccessories{Value: []*hap.Accessory{s.accessory}})
		require.NoError(t, err)
		require.JSONEq(t, string(expected), string(body))
	} else {
		for _, check := range checks {
			check(secure)
		}
	}
	require.NoError(t, secure.Close())
	// Wait until the production handler releases its connection before cleanup.
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.conns) == 0
	}, time.Second, time.Millisecond)
}

// Isolate API configuration globals and app.Init's flags in a child process.
func TestHAPAPIAuthentication(t *testing.T) {
	if os.Getenv("GO2RTC_TEST_HAP_API") == "1" {
		os.Args = []string{os.Args[0], "-c", `{api: {listen: "127.0.0.1:0", username: test, password: test, local_auth: true, base_path: /private, allow_paths: [/private/api/homekit]}}`}
		app.Init()
		api.Init()
		api.HandleFunc("api/homekit", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		apiServer := httptest.NewServer(api.Handler)
		defer apiServer.Close()
		srv, err := serveHAP("127.0.0.1:0", []*mdns.ServiceEntry{{}})
		require.NoError(t, err)
		defer srv.Close()
		client := &http.Client{Timeout: 3 * time.Second}
		getStatus(t, client, "GET", apiServer.URL+"/private/api/homekit", http.StatusUnauthorized)
		req, err := http.NewRequest("GET", apiServer.URL+"/private/api/homekit", nil)
		require.NoError(t, err)
		req.SetBasicAuth("test", "test")
		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.NoError(t, res.Body.Close())
		// HAP routes exist independently of the API's base path and allowlist.
		getStatus(t, client, "GET", "http://"+srv.Addr+"/pair-verify", http.StatusMethodNotAllowed)
		getStatus(t, client, "GET", "http://"+srv.Addr+"/private/api/homekit", http.StatusNotFound)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestHAPAPIAuthentication$", "-test.timeout=15s")
	cmd.Env = append(os.Environ(), "GO2RTC_TEST_HAP_API=1")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
}
