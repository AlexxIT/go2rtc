package homekit

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/AlexxIT/go2rtc/pkg/hap/tlv8"
	"github.com/AlexxIT/go2rtc/pkg/mdns"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func newController(address string, s *server) *hap.Client {
	return &hap.Client{DeviceAddress: address, DeviceID: s.hap.DeviceID,
		ClientID: hap.GenerateUUID(), ClientPrivate: hap.GenerateKey()}
}

func pairController(t *testing.T, client *hap.Client, pin string, success bool) {
	t.Helper()
	err := client.Pair("", pin)
	if success {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
	}
	if client.Conn != nil {
		require.NoError(t, client.Conn.SetReadDeadline(time.Now().Add(time.Second)))
		_, err = io.ReadAll(client.Conn)
		require.NoError(t, err)
		require.NoError(t, client.Close())
	}
}

func timedServer(t *testing.T, s *server, timeout time.Duration) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Handle(w, r, timeout)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestDedicatedPairingPIN(t *testing.T) {
	for _, pin := range []string{"", "19550224", "195-50-224"} {
		_, err := pairingPIN(pin, true, false)
		require.Error(t, err)
		_, err = pairingPIN(pin, true, true)
		require.NoError(t, err, "saved pairings must still start")
		_, err = pairingPIN(pin, false, false)
		require.NoError(t, err, "legacy shared listener keeps its PIN default")
	}
	pin, err := pairingPIN("48261937", true, false)
	require.NoError(t, err)
	require.Equal(t, "482-61-937", pin)
}

func TestCommissioningClosesAndAuthenticatedPairingsStillWork(t *testing.T) {
	s := testCamera(t)
	ts := timedServer(t, s, time.Second)
	address := ts.Listener.Addr().String()
	first := newController(address, s)
	pairController(t, first, s.hap.Pin, true)
	second := newController(address, s)
	pairController(t, second, s.hap.Pin, false)
	require.Nil(t, s.GetPair(second.ClientID))
	verifySession(t, address, first, s, true, func(conn *hap.Conn) {
		// Apple Home adds its other controllers over the already authenticated
		// session; closing initial commissioning must not block that route.
		body := struct {
			Method     byte   `tlv8:"0"`
			Identifier string `tlv8:"1"`
			PublicKey  string `tlv8:"3"`
			State      byte   `tlv8:"6"`
			Permission byte   `tlv8:"11"`
		}{hap.MethodAddPairing, second.ClientID, string(second.ClientPublic()), hap.StateM1, hap.PermissionAdmin}
		req, err := http.NewRequest("POST", "http://"+address+hap.PathPairings, tlv8.MarshalReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", hap.MimeTLV8)
		require.NoError(t, req.Write(conn))
		res, err := http.ReadResponse(bufio.NewReader(conn), req)
		require.NoError(t, err)
		defer res.Body.Close()
		var reply struct {
			State byte `tlv8:"6"`
		}
		require.NoError(t, tlv8.UnmarshalReader(res.Body, res.ContentLength, &reply))
		require.Equal(t, byte(hap.StateM2), reply.State)
	})
	second.DevicePublic = first.DevicePublic
	verifySession(t, address, second, s, true)
}

func TestConcurrentCommissioningAndHandshakeTimeout(t *testing.T) {
	s := testCamera(t)
	ts := timedServer(t, s, 100*time.Millisecond)
	address := ts.Listener.Addr().String()
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	defer conn.Close()
	_, err = fmt.Fprintf(conn, "POST /pair-setup HTTP/1.1\r\nHost: porch.local\r\nContent-Length: 100\r\n\r\n")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.pairingInProgress
	}, time.Second, time.Millisecond)
	other := newController(address, s)
	pairController(t, other, s.hap.Pin, false)
	require.Empty(t, s.GetPair(other.ClientID))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	var data [1]byte
	_, err = conn.Read(data[:])
	require.Error(t, err)
	if e, ok := err.(net.Error); ok {
		require.False(t, e.Timeout(), "server must close stalled setup")
	}
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return !s.pairingInProgress
	}, time.Second, time.Millisecond)
	pairController(t, other, s.hap.Pin, true) // timed-out setup releases its slot
}

func TestVerificationTimeoutAndAuthenticatedSessionDeadline(t *testing.T) {
	s := testCamera(t)
	ts := timedServer(t, s, 100*time.Millisecond)
	address := ts.Listener.Addr().String()
	conn, err := net.Dial("tcp", address)
	require.NoError(t, err)
	_, err = fmt.Fprintf(conn, "POST /pair-verify HTTP/1.1\r\nHost: porch.local\r\nContent-Length: 100\r\n\r\n")
	require.NoError(t, err)
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	var data [1]byte
	_, err = conn.Read(data[:])
	require.Error(t, err)
	if e, ok := err.(net.Error); ok {
		require.False(t, e.Timeout(), "server must close stalled verification")
	}
	require.NoError(t, conn.Close())
	client := newController(address, s)
	pairController(t, client, s.hap.Pin, true)
	verifySession(t, address, client, s, true, func(secure *hap.Conn) {
		time.Sleep(200 * time.Millisecond)
		req, err := http.NewRequest("GET", "http://"+address+hap.PathAccessories, nil)
		require.NoError(t, err)
		require.NoError(t, req.Write(secure))
		res, err := http.ReadResponse(bufio.NewReader(secure), req)
		require.NoError(t, err, "authenticated session retains no handshake deadline")
		defer res.Body.Close()
		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(t, err)
	})
}

func TestDiscoveryMatchesListener(t *testing.T) {
	s := testCamera(t)
	srv, err := serveHAP("127.0.0.1:0", []*mdns.ServiceEntry{s.mdns})
	require.NoError(t, err)
	defer srv.Close()
	require.True(t, s.mdns.IP.Equal(net.ParseIP("127.0.0.1")))
	var allowed, blocked dns.Msg
	mdns.AppendEntry(&allowed, s.mdns, mdns.ServiceHAP, net.ParseIP("127.0.0.1"))
	mdns.AppendEntry(&blocked, s.mdns, mdns.ServiceHAP, net.ParseIP("192.0.2.10"))
	require.NotEmpty(t, allowed.Answer)
	require.Empty(t, blocked.Answer)
	require.Empty(t, blocked.Extra)
	for _, record := range allowed.Extra {
		if record, ok := record.(*dns.SRV); ok {
			require.Equal(t, s.mdns.Port, record.Port)
		}
	}
	ipv6, err := net.Listen("tcp", "[::1]:0")
	if err == nil {
		require.NoError(t, ipv6.Close())
		// A specific IPv6 listener must fail without mutating advertisements.
		entry := &mdns.ServiceEntry{Port: 1234}
		server, err := serveHAP("[::1]:0", []*mdns.ServiceEntry{entry})
		require.Error(t, err)
		require.Nil(t, server)
		require.Equal(t, uint16(1234), entry.Port)
	}
	// A wildcard restart must clear the old interface restriction.
	wildcard, err := serveHAP(":0", []*mdns.ServiceEntry{s.mdns})
	require.NoError(t, err)
	defer wildcard.Close()
	require.Nil(t, s.mdns.IP)
	var anotherInterface dns.Msg
	mdns.AppendEntry(&anotherInterface, s.mdns, mdns.ServiceHAP, net.ParseIP("192.0.2.10"))
	require.NotEmpty(t, anotherInterface.Answer)
}

func TestTransparentProxyEncryptedControlAndCleanup(t *testing.T) {
	upstream := testCamera(t)
	up := timedServer(t, upstream, time.Second)
	upAddress := up.Listener.Addr().String()
	upClient := newController(upAddress, upstream)
	pairController(t, upClient, upstream.hap.Pin, true)
	proxy := &server{
		stream: "proxy", proxyURL: "homekit://synthetic-camera",
		mdns: &mdns.ServiceEntry{Name: "Proxy", Info: map[string]string{}},
	}
	proxy.hap = &hap.Server{Pin: "372-84-619", DeviceID: "22:33:44:55:66:77", DevicePrivate: hap.GenerateKey(), GetClientPublic: proxy.GetPair}
	controller := newController("", proxy)
	proxy.pairings = []string{"client_id=" + controller.ClientID + "&client_public=" + hex.EncodeToString(controller.ClientPublic()) + "&permissions=1"}
	controller.DevicePublic = proxy.hap.ServerPublic()
	proxy.dialProxy = func(rawURL string) (*hap.Client, error) {
		require.Equal(t, proxy.proxyURL, rawURL)
		return &hap.Client{Conn: openVerifiedSession(t, upAddress, upClient, upstream, true)}, nil
	}
	local := timedServer(t, proxy, time.Second)
	address := local.Listener.Addr().String()
	verifySession(t, address, controller, proxy, true, func(secure *hap.Conn) {
		req, err := http.NewRequest("GET", "http://"+address+hap.PathAccessories, nil)
		require.NoError(t, err)
		require.NoError(t, req.Write(secure))
		res, err := http.ReadResponse(bufio.NewReader(secure), req)
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
		expected, err := json.Marshal(hap.JSONAccessories{Value: []*hap.Accessory{upstream.accessory}})
		require.NoError(t, err)
		require.JSONEq(t, string(expected), string(body), "metadata must come from the upstream camera")
	})
	require.Eventually(t, func() bool {
		upstream.mu.Lock()
		defer upstream.mu.Unlock()
		return len(upstream.conns) == 0
	}, time.Second, time.Millisecond, "closing proxy must release the upstream HAP connection")
}

func TestMigratedPublicPINCannotReopenCommissioning(t *testing.T) {
	s := testCamera(t)
	s.privatePINRequired = true
	s.hap.Pin = "195-50-224"
	controller := newController("", s)
	controller.DevicePublic = s.hap.ServerPublic()
	s.pairings = []string{"client_id=" + controller.ClientID + "&client_public=" + hex.EncodeToString(controller.ClientPublic()) + "&permissions=1"}
	ts := timedServer(t, s, time.Second)
	address := ts.Listener.Addr().String()
	verifySession(t, address, controller, s, true, func(secure *hap.Conn) {
		body := struct {
			Method     byte   `tlv8:"0"`
			Identifier string `tlv8:"1"`
			State      byte   `tlv8:"6"`
		}{hap.MethodDeletePairing, controller.ClientID, hap.StateM1}
		req, err := http.NewRequest("POST", "http://"+address+hap.PathPairings, tlv8.MarshalReader(body))
		require.NoError(t, err)
		require.NoError(t, req.Write(secure))
		res, err := http.ReadResponse(bufio.NewReader(secure), req)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, res.Body)
		require.NoError(t, err)
		require.NoError(t, res.Body.Close())
	})
	require.Empty(t, s.GetPair(controller.ClientID))
	attacker := newController(address, s)
	pairController(t, attacker, s.hap.Pin, false)
	require.Empty(t, s.GetPair(attacker.ClientID))
}

func TestProxyDisconnectDuringRequest(t *testing.T) {
	testProxyEOF(t, nil)
}

func TestProxyCompleteResponseBeforeEOF(t *testing.T) {
	testProxyEOF(t, bytes.Repeat([]byte("snapshot"), 512*1024))
}

func testProxyEOF(t *testing.T, responseBody []byte) {
	t.Helper()
	s := testCamera(t)
	s.accessory = nil
	s.proxyURL = "homekit://synthetic-disconnect"
	controller := newController("", s)
	controller.DevicePublic = s.hap.ServerPublic()
	s.pairings = []string{"client_id=" + controller.ClientID + "&client_public=" + hex.EncodeToString(controller.ClientPublic()) + "&permissions=1"}
	upstreamDone := make(chan struct{})
	s.dialProxy = func(string) (*hap.Client, error) {
		a, b := net.Pipe()
		shared := make([]byte, 32)
		client, err := hap.NewConn(a, bufio.NewReadWriter(bufio.NewReader(a), bufio.NewWriter(a)), shared, true)
		if err != nil {
			return nil, err
		}
		peer, err := hap.NewConn(b, bufio.NewReadWriter(bufio.NewReader(b), bufio.NewWriter(b)), shared, false)
		if err != nil {
			return nil, err
		}
		go func() {
			defer close(upstreamDone)
			defer peer.Close()
			_ = peer.SetDeadline(time.Now().Add(3 * time.Second))
			// Drop the upstream connection only after receiving an encrypted request.
			_, _ = http.ReadRequest(bufio.NewReader(peer))
			if responseBody != nil {
				res := &http.Response{StatusCode: http.StatusOK, ProtoMajor: 1, ProtoMinor: 1,
					ContentLength: int64(len(responseBody)), Body: io.NopCloser(bytes.NewReader(responseBody))}
				_ = res.Write(peer)
			}
		}()
		return &hap.Client{Conn: client}, nil
	}
	ts := timedServer(t, s, time.Second)
	address := ts.Listener.Addr().String()
	verifySession(t, address, controller, s, true, func(secure *hap.Conn) {
		path := hap.PathAccessories
		if responseBody != nil {
			path = hap.PathResource
		}
		req, err := http.NewRequest("GET", "http://"+address+path, nil)
		require.NoError(t, err)
		require.NoError(t, req.Write(secure))
		if responseBody != nil {
			time.Sleep(50 * time.Millisecond)
		}
		res, err := http.ReadResponse(bufio.NewReader(secure), req)
		if responseBody == nil {
			require.Error(t, err)
			if e, ok := err.(net.Error); ok {
				require.False(t, e.Timeout(), "camera disconnect must release the response waiter")
			}
		} else {
			require.NoError(t, err)
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.NoError(t, res.Body.Close())
			require.Equal(t, responseBody, body, "last complete camera response must survive EOF")
		}
	})
	select {
	case <-upstreamDone:
	case <-time.After(time.Second):
		t.Fatal("upstream handler did not exit")
	}
}
