package dtls

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
)

func TestRDTRecordChannelIsolation(t *testing.T) {
	c := &DTLSConn{clientBuf: make(chan []byte, 2), serverBuf: make(chan []byte, 2), rdtBuf: make(chan []byte, 2)}
	for _, channel := range []byte{iotcChannelMain, iotcChannelBack, iotcChannelRDT, 9} {
		payload := []byte{channel, 42}
		c.routeDTLSRecord(channel, payload)
		payload[1] = 0 // Router must retain a copy of the reused UDP buffer.
	}
	for name, entry := range map[string]struct {
		ch      chan []byte
		channel byte
	}{
		"main": {c.clientBuf, iotcChannelMain}, "back": {c.serverBuf, iotcChannelBack}, "rdt": {c.rdtBuf, iotcChannelRDT},
	} {
		if len(entry.ch) != 1 {
			t.Fatalf("%s queue received %d records, want exactly one", name, len(entry.ch))
		}
		if got := <-entry.ch; !bytes.Equal(got, []byte{entry.channel, 42}) {
			t.Fatalf("%s received another channel or a mutated payload", name)
		}
	}
	if c.rdtReceived.Load() != 1 {
		t.Fatal("RDT diagnostic counter included another channel")
	}
	// Before registration and after teardown, channel 10 is ignored; AV survives.
	c.rdtBuf = nil
	c.routeDTLSRecord(iotcChannelRDT, []byte{99})
	if len(c.clientBuf) != 0 || len(c.serverBuf) != 0 {
		t.Fatal("unregistered RDT record leaked into AV queues")
	}
}

func TestRDTHandshakeDiagnosticsNeverExposeUpstreamText(t *testing.T) {
	message := safeRDTHandshakeFailure(errors.New("PRIVATE_ENR_AND_PEER_DATA"), 7).Error()
	if strings.Contains(message, "PRIVATE_ENR_AND_PEER_DATA") || !strings.Contains(message, "incoming_datagrams=7") {
		t.Fatal("unsafe or missing handshake diagnostics")
	}
	if message := safeRDTHandshakeFailure(context.DeadlineExceeded, 0).Error(); !strings.Contains(message, "class=timeout") {
		t.Fatal("timeout stage was not classified")
	}
	message = safeRDTHandshakeFailure(testRDTAlert{}, 2).Error()
	if !strings.Contains(message, "alert_level=2") || !strings.Contains(message, "alert_description=115") || strings.Contains(message, "PRIVATE") {
		t.Fatal("numeric peer alert diagnostics missing or unsafe")
	}
	if string(buildDTLSConfig([]byte{1}, false).PSKIdentityHint) != "AUTHPWD_admin" {
		t.Fatal("existing AV identity changed")
	}
}

type testRDTAlert struct{}

func (testRDTAlert) Error() string              { return "PRIVATE_UPSTREAM_DETAIL" }
func (testRDTAlert) IsFatalOrCloseNotify() bool { return true }
func (testRDTAlert) Marshal() ([]byte, error)   { return []byte{2, 115}, nil }

func TestRDTHandshakeCleanupCancelsSeparateContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &DTLSConn{rdtCancel: cancel, rdtBuf: make(chan []byte, 1), clientBuf: make(chan []byte, 1)}
	c.closeRDTHandshake()
	if ctx.Err() == nil || c.rdtBuf != nil || c.rdtCancel != nil {
		t.Fatal("RDT teardown did not cancel and unregister its context/queue")
	}
	c.routeDTLSRecord(iotcChannelMain, []byte{1})
	if len(c.clientBuf) != 1 {
		t.Fatal("RDT teardown affected the AV receive queue")
	}
}

func TestRDTRemoteAddressSnapshotIsIndependent(t *testing.T) {
	c := &DTLSConn{addr: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 32761}}
	snapshot := c.remoteAddrSnapshot()
	snapshot.Port = 1
	snapshot.IP[0] = 42
	if current := c.remoteAddrSnapshot(); current.Port != 32761 || !current.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatal("caller mutated the transport address through its snapshot")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			c.addrMu.Lock()
			c.addr.Port++
			c.addrMu.Unlock()
		}
	}()
	for range 1000 {
		_ = c.remoteAddrSnapshot()
	}
	wg.Wait()
}

func TestRDTReuseRequiresFreshAuthenticatedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &DTLSConn{ctx: ctx, conn: new(net.UDPConn), rdtUsed: true}
	// No transport writer is configured. A retry must fail at preflight rather
	// than sending a new DTLS handshake or claiming the old channel is reusable.
	if err := c.ProbeRDTHandshake(make([]byte, 32)); err == nil || err.Error() != "RDT requires a fresh authenticated session" {
		t.Fatalf("reuse result=%v", err)
	}
}
