package dtls

import (
	"context"
	"encoding/hex"
	"net"
	"testing"
	"time"
)

func TestCC51SessionCloseGolden(t *testing.T) {
	c := &DTLSConn{uid: "ABCDEFGHIJKLMNOPQRST", authKey: "12345678", sid: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	// Independent firmware-layout vector; MAC computed with Python hmac/sha1.
	want := "51cc00000213240000000000000000000403020101020304050607085014d8c2a76060e6f07c41067d6cc463d1f7c8d9"
	if got := hex.EncodeToString(c.msgSessionCloseCC51(time.Unix(0x01020304, 0))); got != want {
		t.Fatalf("close frame=%s", got)
	}
}

func TestCC51CloseNotifiesPeerBeforeSocketReleaseAndIsIdempotent(t *testing.T) {
	peer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	local, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &DTLSConn{conn: local, addr: peer.LocalAddr().(*net.UDPAddr), ctx: ctx, cancel: cancel, isCC51: true, uid: "ABCDEFGHIJKLMNOPQRST", authKey: "12345678", sid: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	b := make([]byte, 256)
	for range 3 {
		n, _, err := peer.ReadFromUDP(b)
		if err != nil || n != 48 || b[5] != 0x13 || string(b[20:28]) != string(c.sid) {
			t.Fatalf("missing SID-scoped close: n=%d err=%v", n, err)
		}
	}
	_ = peer.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
	if _, _, err = peer.ReadFromUDP(b); err == nil {
		t.Fatal("idempotent close sent another notification")
	}
}
