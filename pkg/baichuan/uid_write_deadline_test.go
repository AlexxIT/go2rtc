package baichuan

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"testing"
	"time"
)

func TestUIDConnExpiredWriteDeadlinePreservesReceiveAndMaintenance(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	if _, err := conn.Write([]byte("accepted")); err != nil {
		t.Fatal(err)
	}
	first := readUIDPacket(t, camera, time.Second)
	if err := conn.SetWriteDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := conn.Write([]byte("expired")); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expired admission = %d, %v", n, err)
	}
	sendUIDData(t, camera, conn, 0, []byte("A"))
	sendUIDData(t, camera, conn, 2, []byte("C"))
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	data := make([]byte, 1)
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "A" {
		t.Fatalf("read after write expiry = %q, %v", data, err)
	}
	ackCount, retransmitted := 0, false
	for ackCount < 2 || !retransmitted {
		packet := readUIDPacket(t, camera, time.Second)
		switch packet.magic {
		case uidMagicData:
			if packet.packetID != first.packetID || !bytes.Equal(packet.payload, first.payload) {
				t.Fatalf("accepted retransmit changed: %+v", packet)
			}
			retransmitted = true
		case uidMagicAck:
			if packet.packetID == 0 && bytes.Equal(packet.payload, []byte{0, 1}) {
				ackCount++
			}
		}
	}
	select {
	case <-conn.done:
		t.Fatalf("application deadline closed transport: %v", conn.readError())
	default:
	}
	if err := conn.SetWriteDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("resumed")); err != nil {
		t.Fatal(err)
	}
}

func TestUIDConnActiveDatagramDeadlineOwnership(t *testing.T) {
	for _, application := range []bool{false, true} {
		name := "maintenance"
		if application {
			name = "application"
		}
		t.Run(name, func(t *testing.T) {
			conn, _ := newTestUIDConnWithoutWorkers(t, 100*time.Millisecond)
			conn.socketWriteMu.Lock()
			defer conn.socketWriteMu.Unlock()
			// Hold the writer ownership used by writeDatagram while exercising
			// concurrent setters and the real UDP socket's deadline behavior.
			if err := conn.beginDatagram(application); err != nil {
				t.Fatal(err)
			}
			bound := conn.socketDeadline
			set := func(value time.Time) {
				t.Helper()
				result := make(chan error, 1)
				go func() { result <- conn.SetWriteDeadline(value) }()
				select {
				case err := <-result:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("deadline setter waited for socket writer")
				}
			}
			set(time.Now().Add(-time.Second))
			_, err := conn.conn.WriteToUDPAddrPort([]byte("expired"), conn.remote)
			if application && !errors.Is(err, os.ErrDeadlineExceeded) || !application && err != nil {
				t.Fatalf("expired application deadline affected %s write: %v", name, err)
			}
			set(bound.Add(time.Hour))
			set(time.Time{})
			if _, err = conn.conn.WriteToUDPAddrPort([]byte("cleared"), conn.remote); err != nil {
				t.Fatalf("clear did not restore active transport bound: %v", err)
			}
			<-time.After(time.Until(bound) + time.Millisecond)
			if _, err = conn.conn.WriteToUDPAddrPort([]byte("bounded"), conn.remote); !errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatalf("change/clear erased %s transport bound: %v", name, err)
			}
		})
	}
}

func TestUIDConnMaintenanceInstallsIndependentDeadline(t *testing.T) {
	conn, camera := newTestUIDConnWithoutWorkers(t, time.Second)
	_ = conn.SetWriteDeadline(time.Now().Add(-time.Second))
	if err := conn.writeDatagram([]byte("maintenance"), false); err != nil {
		t.Fatalf("maintenance inherited expired application deadline: %v", err)
	}
	_ = camera.SetReadDeadline(time.Now().Add(time.Second))
	data := make([]byte, uidMTU)
	if n, _, err := camera.ReadFromUDPAddrPort(data); err != nil || string(data[:n]) != "maintenance" {
		t.Fatalf("maintenance packet = %q, %v", data[:n], err)
	}
	if err := conn.writeDatagram([]byte("application"), true); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("application ignored expired deadline: %v", err)
	}
	_ = conn.SetWriteDeadline(time.Time{})
	if err := conn.writeDatagram([]byte("application"), true); err != nil {
		t.Fatalf("application did not resume: %v", err)
	}
}

func newTestUIDConnWithoutWorkers(t *testing.T, timeout time.Duration) (*uidConn, *net.UDPConn) {
	t.Helper()
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	camera, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		_ = client.Close()
		t.Fatal(err)
	}
	conn := &uidConn{conn: client, remote: camera.LocalAddr().(*net.UDPAddr).AddrPort(),
		clientID: 1001, cameraID: 2002, timeout: timeout, done: make(chan struct{}),
		readQueue: make(chan uidChunk, uidReadQueue), sendWake: make(chan struct{}, 1)}
	conn.readDeadline.init()
	conn.writeDeadline.init()
	conn.pool.New = func() any { return new(uidBuffer) }
	t.Cleanup(func() { _ = conn.Close(); _ = camera.Close() })
	return conn, camera
}
