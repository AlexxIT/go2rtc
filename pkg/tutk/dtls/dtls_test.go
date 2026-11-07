package dtls

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/pion/dtls/v3"
)

// handshake runs the client config against a pion server restricted to the
// given suites and returns the negotiated suite.
func handshake(t *testing.T, server *dtls.Config, psk []byte) (dtls.CipherSuiteID, error) {
	t.Helper()
	ln, err := dtls.Listen("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}, server)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err == nil {
			_ = c.(*dtls.Conn).HandshakeContext(context.Background())
			time.Sleep(200 * time.Millisecond)
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := buildDTLSConfig(psk, false)
	cfg.FlightInterval = 50 * time.Millisecond
	conn, err := dtls.Dial("udp", ln.Addr().(*net.UDPAddr), cfg)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if err = conn.HandshakeContext(ctx); err != nil {
		return 0, err
	}
	st, _ := conn.ConnectionState()
	return st.CipherSuiteID, nil
}

func serverCfg(psk []byte) *dtls.Config {
	return &dtls.Config{
		PSK:                  func([]byte) ([]byte, error) { return psk, nil },
		PSKIdentityHint:      []byte("AUTHPWD_admin"),
		ExtendedMasterSecret: dtls.DisableExtendedMasterSecret,
	}
}

// Older firmware (Wyze Video Doorbell v1) only supports AES-128-CBC.
func TestClientHandshakeAESOnlyServer(t *testing.T) {
	psk := DerivePSK("testenr123456789")
	s := serverCfg(psk)
	s.CipherSuites = []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_CBC_SHA256}
	id, err := handshake(t, s, psk)
	if err != nil || id != dtls.TLS_PSK_WITH_AES_128_CBC_SHA256 {
		t.Fatalf("got %v %v", id, err)
	}
}

// Newer firmware must keep negotiating ChaCha20.
func TestClientHandshakeChaChaServer(t *testing.T) {
	psk := DerivePSK("testenr123456789")
	s := serverCfg(psk)
	s.CustomCipherSuites = CustomCipherSuites
	s.CipherSuites = []dtls.CipherSuiteID{}
	id, err := handshake(t, s, psk)
	if err != nil || id != CipherSuiteID_CCAC {
		t.Fatalf("got %v %v", id, err)
	}
}
