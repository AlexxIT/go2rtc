package baichuan

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestUIDDiscoveryRejectsInvalidConfig(t *testing.T) {
	for _, uid := range []string{"", "bad uid", string(make([]byte, 65))} {
		if _, err := discoverUID(context.Background(), uid, "", "", time.Second); err == nil {
			t.Fatalf("accepted UID of length %d", len(uid))
		}
	}
	if _, err := discoverUID(context.Background(), "valid", "", "", 0); err == nil {
		t.Fatal("accepted zero timeout")
	}
	if _, err := discoverUID(context.Background(), "valid", "not-an-address", "", time.Second); err == nil {
		t.Fatal("accepted invalid local address")
	}
	if _, err := discoverUID(context.Background(), "valid", "", "127.0.0.1", time.Second); err == nil {
		t.Fatal("accepted invalid broadcast address")
	}
}

func TestUIDBroadcastSocketOption(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = enableUIDBroadcast(conn); err != nil {
		t.Fatal(err)
	}
}

func TestUIDBroadcastSelectionRejectsInactiveAddress(t *testing.T) {
	if _, err := uidBroadcasts(netip.MustParseAddr("192.0.2.1"), netip.Addr{}); err == nil {
		t.Fatal("accepted inactive UID local address")
	}
}

func TestUIDRoutedBroadcastOverride(t *testing.T) {
	local := netip.MustParseAddr("192.0.2.211")
	want := netip.MustParseAddr("198.51.100.255")
	broadcasts, err := uidBroadcasts(local, want)
	if err != nil || len(broadcasts) != 1 || broadcasts[0] != want {
		t.Fatalf("unexpected UID broadcast override: %v %v", broadcasts, err)
	}
}

func TestUIDDiscoveryResponseMatchesTransaction(t *testing.T) {
	const clientID int32 = 71
	const transaction uint32 = 29
	payload, err := xml.Marshal(uidEnvelope{Response: &uidResponse{CID: clientID, DID: 83, Rsp: 0}})
	if err != nil {
		t.Fatal(err)
	}
	payload = append([]byte(xml.Header), payload...)
	xorUID(payload, payload, transaction)
	packet, err := marshalUIDDiscovery(transaction, payload)
	if err != nil {
		t.Fatal(err)
	}
	remote := netip.MustParseAddrPort("203.0.113.67:2015")
	if _, ok := parseUIDResponse(append([]byte(nil), packet...), remote, clientID, transaction+1); ok {
		t.Fatal("accepted UID discovery response for another transaction")
	}
	if _, ok := parseUIDResponse(append([]byte(nil), packet...), remote, clientID+1, transaction); ok {
		t.Fatal("accepted UID discovery response for another client")
	}
	if cameraID, ok := parseUIDResponse(packet, remote, clientID, transaction); !ok || cameraID != 83 {
		t.Fatalf("valid UID response = %d, %t", cameraID, ok)
	}
}

func TestUIDDiscoveryDatagramSize(t *testing.T) {
	for _, size := range []int{uidMTU, uidMTU + 1, 4096} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			camera, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer camera.Close()
			const transaction = 29
			payload := []byte(`<P2P><D2C_C_R><cid>71</cid><did>83</did><rsp>0</rsp></D2C_C_R></P2P>`)
			for len(payload) < uidMTU-uidDiscoveryHeader {
				payload = append(payload, ' ')
			}
			xorUID(payload, payload, transaction)
			packet, err := marshalUIDDiscovery(transaction, payload)
			if err != nil {
				t.Fatal(err)
			}
			packet = append(packet, make([]byte, size-len(packet))...)
			if _, err = camera.WriteToUDP(packet, conn.LocalAddr().(*net.UDPAddr)); err != nil {
				t.Fatal(err)
			}
			if size > uidMTU {
				// Distinct valid response proves the oversized prefix was ignored.
				valid := []byte(`<P2P><D2C_C_R><cid>71</cid><did>84</did><rsp>0</rsp></D2C_C_R></P2P>`)
				xorUID(valid, valid, transaction)
				packet, _ = marshalUIDDiscovery(transaction, valid)
				if _, err = camera.WriteToUDP(packet, conn.LocalAddr().(*net.UDPAddr)); err != nil {
					t.Fatal(err)
				}
			}
			_ = conn.SetReadDeadline(time.Now().Add(time.Second))
			b := make([]byte, uidMTU+1)
			n, _, err := readUIDDatagram(conn, b)
			if err != nil {
				t.Fatal(err)
			}
			want := int32(83)
			if size > uidMTU {
				want = 84
			}
			remote := netip.MustParseAddrPort("203.0.113.67:2015")
			if id, ok := parseUIDResponse(b[:n], remote, 71, transaction); !ok || id != want {
				t.Fatalf("size %d: camera=%d valid=%t", size, id, ok)
			}
		})
	}
}

func TestUIDDatagramReadErrors(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	b := make([]byte, uidMTU+1)
	_ = conn.SetReadDeadline(time.Now().Add(-time.Second))
	if _, _, err = readUIDDatagram(conn, b); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("read deadline: %v", err)
	}
	_ = conn.Close()
	if _, _, err = readUIDDatagram(conn, b); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("closed socket: %v", err)
	}
}
