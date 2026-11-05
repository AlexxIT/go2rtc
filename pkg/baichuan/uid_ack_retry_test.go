package baichuan

import (
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"testing"
	"time"
)

func TestUIDConnRetriesLostGapAcknowledgement(t *testing.T) {
	conn, camera := newTestUIDConn(t, time.Second)
	sendUIDData(t, camera, conn, 0, []byte("A"))
	data := make([]byte, 1)
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "A" {
		t.Fatalf("first delivery = %q, %v", data, err)
	}
	sendUIDData(t, camera, conn, 2, []byte("C"))
	first := readUIDPacket(t, camera, time.Second)
	if len(first.payload) == 0 {
		first = readUIDPacket(t, camera, time.Second)
	}
	if first.magic != uidMagicAck || first.packetID != 0 || !bytes.Equal(first.payload, []byte{0, 1}) {
		t.Fatalf("gap ACK = %+v", first)
	}
	// Drop this ACK. With no more inbound traffic, maintenance must repeat it.
	retry := readUIDPacket(t, camera, time.Second)
	if retry.magic != first.magic || retry.packetID != first.packetID ||
		!bytes.Equal(retry.payload, first.payload) {
		t.Fatalf("gap retry changed: first=%+v retry=%+v", first, retry)
	}
	sendUIDData(t, camera, conn, 1, []byte("B"))
	data = make([]byte, 2)
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "BC" {
		t.Fatalf("gap delivery = %q, %v", data, err)
	}
	for {
		ack := readUIDPacket(t, camera, time.Second)
		if ack.packetID == 2 {
			if len(ack.payload) != 0 {
				t.Fatalf("resolved ACK = %+v", ack)
			}
			break
		}
	}
	_ = camera.SetReadDeadline(time.Now().Add(2 * uidRetransmit))
	if _, _, err := camera.ReadFromUDPAddrPort(make([]byte, uidMTU)); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("resolved gap kept retrying: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUIDConnAcknowledgesMissingFirstPacket(t *testing.T) {
	conn, camera := newTestUIDConnWithoutWorkers(t, time.Second)
	// Both copies arrive before maintenance, with no contiguous delivery yet.
	b := conn.getBuffer()
	b[0] = 'B'
	if !conn.handleUIDData(uidPacket{packetID: 1, payload: b[:1]}, b) {
		t.Fatal("first out-of-order packet rejected")
	}
	duplicate := conn.getBuffer()
	if conn.handleUIDData(uidPacket{packetID: 1, payload: duplicate[:1]}, duplicate) {
		t.Fatal("duplicate retained")
	}
	conn.putBuffer(duplicate)
	if err := conn.writeUIDAck(); err != nil {
		t.Fatal(err)
	}
	ack := readUIDPacket(t, camera, time.Second)
	if ack.magic != uidMagicAck || ack.packetID != math.MaxUint32 || !bytes.Equal(ack.payload, []byte{0, 1}) {
		t.Fatalf("missing-first ACK = %+v", ack)
	}
	b = conn.getBuffer()
	b[0] = 'A'
	if !conn.handleUIDData(uidPacket{packetID: 0, payload: b[:1]}, b) {
		t.Fatal("missing packet rejected")
	}
	data := make([]byte, 2)
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "AB" {
		t.Fatalf("first gap delivery = %q, %v", data, err)
	}
}

func TestUIDConnFullWindowSelectiveAcknowledgement(t *testing.T) {
	for _, first := range []uint32{0, math.MaxUint32 - 31} {
		conn := &uidConn{clientID: 1001, nextSend: first + uidSendWindow, sendCount: uidSendWindow,
			sendWake: make(chan struct{}, 1)}
		var selective [uidSendWindow]byte
		for i := range selective {
			id := first + uint32(i)
			conn.sendSlots[id%uidSendWindow] = uidSendSlot{packetID: id, buffer: new(uidBuffer), used: true}
			if i > 0 {
				selective[i] = 1
			}
		}
		ack := uidPacket{connectionID: conn.clientID, packetID: first - 1, payload: selective[:]}
		conn.handleUIDAck(ack)
		conn.handleUIDAck(ack) // Duplicate selective ACKs cannot free the missing slot.
		if conn.sendCount != 1 {
			t.Fatalf("first=%d: pending=%d", first, conn.sendCount)
		}
		for _, slot := range conn.sendSlots {
			if slot.used && slot.packetID != first {
				t.Fatalf("first=%d: retained wrong slot %d", first, slot.packetID)
			}
		}
		if !conn.sendSlots[first%uidSendWindow].used {
			t.Fatalf("first=%d: missing slot was released", first)
		}
		conn.handleUIDAck(uidPacket{connectionID: conn.clientID, packetID: first})
		if conn.sendCount != 0 {
			t.Fatalf("first=%d: cumulative ACK left %d slots", first, conn.sendCount)
		}
	}
}

func TestUIDConnRejectsStaleFullWindowAcknowledgement(t *testing.T) {
	for _, distance := range []uint32{uidSendWindow + 1, uidSendWindow + 2} {
		conn := &uidConn{clientID: 1001, nextSend: uidSendWindow, sendCount: 1}
		conn.sendSlots[1] = uidSendSlot{packetID: 1, used: true}
		selective := bytes.Repeat([]byte{1}, uidSendWindow)
		conn.handleUIDAck(uidPacket{connectionID: conn.clientID, packetID: conn.nextSend - distance, payload: selective})
		if conn.sendCount != 1 || !conn.sendSlots[1].used {
			t.Fatalf("stale distance %d released a slot", distance)
		}
	}
}
