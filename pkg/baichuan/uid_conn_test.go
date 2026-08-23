package baichuan

import (
	"errors"
	"io"
	"math"
	"net"
	"os"
	"testing"
	"time"
)

func TestUIDConnOrdersDataAndAcknowledges(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	sendUIDData(t, camera, conn, 1, []byte("B"))
	sendUIDData(t, camera, conn, 0, []byte("A"))
	b := make([]byte, 2)
	if _, err := io.ReadFull(conn, b); err != nil {
		t.Fatal(err)
	}
	if string(b) != "AB" {
		t.Fatalf("unexpected ordered data: %q", b)
	}
	packet := readUIDPacket(t, camera, time.Second)
	if packet.magic != uidMagicAck || packet.connectionID != conn.cameraID ||
		packet.packetID != 1 || len(packet.payload) != 0 {
		t.Fatalf("unexpected ACK: %+v", packet)
	}
}

func TestUIDConnDoesNotRedeliverDuplicateData(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	sendUIDData(t, camera, conn, 0, []byte("A"))
	b := make([]byte, 1)
	if _, err := io.ReadFull(conn, b); err != nil || string(b) != "A" {
		t.Fatalf("unexpected first delivery: %q %v", b, err)
	}
	sendUIDData(t, camera, conn, 0, []byte("A"))
	if err := conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(b); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("duplicate reached reader: %v", err)
	}
}

func TestUIDConnConsumesEmptyData(t *testing.T) {
	conn, _ := newTestUIDConn(t, time.Second)
	b := conn.getBuffer()
	if !conn.handleUIDData(uidPacket{packetID: 0, payload: b[:0]}, b) {
		t.Fatal("empty packet buffer was not consumed")
	}
	conn.receiveMu.Lock()
	next := conn.nextReceive
	conn.receiveMu.Unlock()
	if next != 1 || len(conn.readQueue) != 0 {
		t.Fatalf("empty packet reached reader: next=%d queued=%d", next, len(conn.readQueue))
	}
}

func TestUIDConnDeadlinesAndClose(t *testing.T) {
	conn, _ := newTestUIDConn(t, time.Second)
	if err := conn.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("unexpected read deadline error: %v", err)
	}
	if err := conn.SetWriteDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, (uidMTU-uidDataHeader)*(uidSendWindow+1))
	n, err := conn.Write(payload)
	if !errors.Is(err, os.ErrDeadlineExceeded) || n != (uidMTU-uidDataHeader)*uidSendWindow {
		t.Fatalf("unexpected bounded write: n=%d err=%v", n, err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected closed read: %v", err)
	}
	if _, err = conn.Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("unexpected closed write: %v", err)
	}
}

func TestUIDConnIgnoresWrongEndpoint(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	spoof, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer spoof.Close()
	packet, err := marshalUIDData(conn.clientID, 0, []byte("spoof"), uidMTU-uidDataHeader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = spoof.WriteToUDPAddrPort(packet, conn.conn.LocalAddr().(*net.UDPAddr).AddrPort()); err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, []byte{1, 2, 3, 4})
	_ = conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if _, err = conn.Read(make([]byte, 1)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("spoofed packet reached reader: %v", err)
	}
}

func TestUIDConnSequenceWrap(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	conn.sendMu.Lock()
	conn.nextSend = math.MaxUint32
	conn.sendMu.Unlock()
	payload := make([]byte, uidMTU-uidDataHeader+1)
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	first := readUIDPacket(t, camera, time.Second)
	second := readUIDPacket(t, camera, time.Second)
	if first.packetID != math.MaxUint32 || second.packetID != 0 {
		t.Fatalf("send sequence did not wrap: %d %d", first.packetID, second.packetID)
	}
	ack, err := marshalUIDAck(conn.clientID, 0, nil, uidSendWindow)
	if err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, ack)
	waitFor(t, time.Second, func() bool {
		conn.sendMu.Lock()
		count := conn.sendCount
		conn.sendMu.Unlock()
		return count == 0
	})

	conn.receiveMu.Lock()
	conn.nextReceive = math.MaxUint32
	conn.receiveMu.Unlock()
	sendUIDData(t, camera, conn, 0, []byte("B"))
	sendUIDData(t, camera, conn, math.MaxUint32, []byte("A"))
	b := make([]byte, 2)
	if _, err = io.ReadFull(conn, b); err != nil || string(b) != "AB" {
		t.Fatalf("receive sequence did not wrap: %q %v", b, err)
	}
}

func TestUIDConnReadQueueOverflowIsTerminal(t *testing.T) {
	conn, _ := newTestUIDConn(t, time.Second)
	for packetID := uint32(0); packetID <= uidReadQueue; packetID++ {
		b := conn.getBuffer()
		packet, err := encodeUIDData(b[:], conn.clientID, packetID, []byte{1}, 1)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parseUIDPacket(packet, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !conn.handleUIDData(parsed, b) {
			conn.putBuffer(b)
		}
	}
	select {
	case <-conn.done:
	case <-time.After(time.Second):
		t.Fatal("overflow did not close connection")
	}
	if !errors.Is(conn.readError(), errUIDReadOverflow) {
		t.Fatalf("unexpected overflow error: %v", conn.readError())
	}
}

func newTestUIDConn(t *testing.T, timeout time.Duration) (*uidConn, *net.UDPConn) {
	t.Helper()
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	camera, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	discovery := &uidDiscovery{
		conn: client, remote: camera.LocalAddr().(*net.UDPAddr).AddrPort(),
		clientID: 1001, cameraID: 2002,
	}
	conn, err := newUIDConn(discovery, timeout)
	if err != nil {
		camera.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		_ = camera.Close()
	})
	return conn, camera
}

func sendUIDData(t *testing.T, camera *net.UDPConn, conn *uidConn, packetID uint32, payload []byte) {
	t.Helper()
	packet, err := marshalUIDData(conn.clientID, packetID, payload, uidMTU-uidDataHeader)
	if err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, packet)
}

func writeUIDPacket(t *testing.T, camera *net.UDPConn, conn *uidConn, packet []byte) {
	t.Helper()
	if _, err := camera.WriteToUDPAddrPort(packet, conn.conn.LocalAddr().(*net.UDPAddr).AddrPort()); err != nil {
		t.Fatal(err)
	}
}

func readUIDPacket(t *testing.T, camera *net.UDPConn, timeout time.Duration) uidPacket {
	t.Helper()
	if err := camera.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, uidMTU)
	n, _, err := camera.ReadFromUDPAddrPort(b)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := parseUIDPacket(b[:n], uidMTU-uidDataHeader)
	if err != nil {
		t.Fatal(err)
	}
	return packet
}

func waitFor(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !ready() {
		if !time.Now().Before(deadline) {
			t.Fatal("condition timed out")
		}
		time.Sleep(time.Millisecond)
	}
}
