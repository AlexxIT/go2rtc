package baichuan

import (
	"bytes"
	"encoding/xml"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUIDConnAcknowledgementWaitsForInitialSend(t *testing.T) {
	conn, camera := newTestUIDConnWithoutWorkers(t, time.Second)
	conn.socketWriter <- struct{}{}
	writerHeld := true
	defer func() {
		if writerHeld {
			<-conn.socketWriter
		}
	}()
	written := make(chan error, 1)
	go func() {
		_, err := conn.Write([]byte("request"))
		written <- err
	}()
	waitFor(t, time.Second, func() bool {
		conn.sendMu.Lock()
		defer conn.sendMu.Unlock()
		return conn.sendSlots[0].writing
	})
	ack := uidPacket{connectionID: conn.clientID, packetID: 0}
	conn.handleUIDAck(ack)
	conn.handleUIDAck(ack)
	if err := conn.retransmitUID(time.Now().Add(uidRetransmit)); err != nil {
		t.Fatal(err)
	}
	conn.sendMu.Lock()
	slot := conn.sendSlots[0]
	pending := conn.sendCount
	conn.sendMu.Unlock()
	if pending != 1 || !slot.used || !slot.writing || !slot.acked || string(slot.packet[uidDataHeader:]) != "request" {
		t.Fatalf("ACK did not preserve in-flight ownership: pending=%d slot=%+v", pending, slot)
	}
	<-conn.socketWriter
	writerHeld = false
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	packet := readUIDPacket(t, camera, time.Second)
	if string(packet.payload) != "request" || conn.sendCount != 0 || conn.sendSlots[0].used {
		t.Fatalf("send completion did not release acknowledged bytes: packet=%+v pending=%d", packet, conn.sendCount)
	}
}

func TestUIDConnRetransmitsUntilAcknowledged(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	if _, err := conn.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	first := readUIDPacket(t, camera, time.Second)
	second := readUIDPacket(t, camera, time.Second)
	if first.magic != uidMagicData || second.magic != uidMagicData ||
		first.packetID != 0 || second.packetID != 0 ||
		!bytes.Equal(first.payload, second.payload) {
		t.Fatalf("unexpected retransmission: first=%+v second=%+v", first, second)
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
}

func TestUIDConnSelectiveAcknowledgement(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	payload := make([]byte, (uidMTU-uidDataHeader)*2+1)
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_ = readUIDPacket(t, camera, time.Second)
	}
	ack, err := marshalUIDAck(conn.clientID, 0, []byte{0, 1}, uidSendWindow)
	if err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, ack)
	waitFor(t, time.Second, func() bool {
		conn.sendMu.Lock()
		defer conn.sendMu.Unlock()
		return conn.sendCount == 1 && conn.sendSlots[1].used
	})
	ack, err = marshalUIDAck(conn.clientID, 1, nil, uidSendWindow)
	if err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, ack)
	waitFor(t, time.Second, func() bool {
		conn.sendMu.Lock()
		defer conn.sendMu.Unlock()
		return conn.sendCount == 0
	})
}

func TestUIDConnSendWindowPreservesUnacknowledgedSlot(t *testing.T) {
	conn := &uidConn{done: make(chan struct{}), sendWake: make(chan struct{}, 1)}
	conn.writeDeadline.init()
	conn.nextSend = uidSendWindow + 1
	conn.sendCount = 1
	conn.sendSlots[1] = uidSendSlot{packetID: 1, used: true}
	conn.writeDeadline.set(time.Now().Add(-time.Second))
	if packetID, err := conn.reserveSend(); err != os.ErrDeadlineExceeded {
		t.Fatalf("reserved packet %d over unacknowledged slot: %v", packetID, err)
	}
	if conn.nextSend != uidSendWindow+1 || conn.sendCount != 1 {
		t.Fatalf("send window changed: next=%d pending=%d", conn.nextSend, conn.sendCount)
	}
	conn.writeDeadline.set(time.Time{})
	conn.sendMu.Lock()
	conn.releaseSendSlot(&conn.sendSlots[1])
	conn.sendMu.Unlock()
	packetID, err := conn.reserveSend()
	if err != nil || packetID != uidSendWindow+1 {
		t.Fatalf("reservation after release = %d, %v", packetID, err)
	}
	conn.cancelSendReservation()
}

func TestUIDConnRejectsFutureAcknowledgement(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	if _, err := conn.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	_ = readUIDPacket(t, camera, time.Second)
	ack, err := marshalUIDAck(conn.clientID, 1, nil, uidSendWindow)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseUIDPacket(ack, uidSendWindow)
	if err != nil {
		t.Fatal(err)
	}
	conn.handleUIDAck(parsed)
	conn.sendMu.Lock()
	pending := conn.sendCount
	conn.sendMu.Unlock()
	if pending != 1 {
		t.Fatalf("future ACK changed send window: pending=%d", pending)
	}
	ack, err = marshalUIDAck(conn.clientID, 0, nil, uidSendWindow)
	if err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, ack)
	waitFor(t, time.Second, func() bool {
		conn.sendMu.Lock()
		defer conn.sendMu.Unlock()
		return conn.sendCount == 0
	})
}

func TestUIDConnAcknowledgementTimeoutIsTerminal(t *testing.T) {
	conn, _ := newTestUIDConn(t, 120*time.Millisecond)
	if _, err := conn.Write([]byte("request")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-conn.done:
	case <-time.After(time.Second):
		t.Fatal("missing acknowledgement did not close transport")
	}
	if err := conn.readError(); err == nil || !strings.Contains(err.Error(), "acknowledgement timed out") {
		t.Fatalf("unexpected terminal error: %v", err)
	}
}

func TestUIDConnCameraDisconnectIsTerminal(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	payload, err := xml.Marshal(struct {
		XMLName    xml.Name `xml:"P2P"`
		Disconnect struct {
			CID int32 `xml:"cid"`
			DID int32 `xml:"did"`
		} `xml:"D2C_DISC"`
	}{Disconnect: struct {
		CID int32 `xml:"cid"`
		DID int32 `xml:"did"`
	}{conn.clientID, conn.cameraID}})
	if err != nil {
		t.Fatal(err)
	}
	const transaction = 17
	xorUID(payload, payload, transaction)
	packet, err := marshalUIDDiscovery(transaction, payload)
	if err != nil {
		t.Fatal(err)
	}
	writeUIDPacket(t, camera, conn, packet)
	select {
	case <-conn.done:
	case <-time.After(time.Second):
		t.Fatal("camera disconnect did not close transport")
	}
	if err = conn.readError(); err == nil || !strings.Contains(err.Error(), "camera closed") {
		t.Fatalf("unexpected terminal error: %v", err)
	}
}
