package dtls

import (
	"encoding/hex"
	"testing"
)

func TestRDTNativeHelloVectors(t *testing.T) {
	hello, err := rdtHello(1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeRDTPacket(hello)
	if err != nil || hex.EncodeToString(encoded) != "5a97c2f101050000000000000000000000010000" {
		t.Fatal("native HELLO vector mismatch")
	}
	peerHello, _ := rdtHello(7)
	ack, err := rdtAck(peerHello, 99)
	encoded, encodeErr := encodeRDTPacket(ack)
	if err != nil || encodeErr != nil || hex.EncodeToString(encoded) != "5a97c2f141050000000000000000000000070000" {
		t.Fatal("HELLO_ACK must echo the incoming ID")
	}
	for _, invalid := range []byte{0, 255} {
		if _, err := rdtHello(invalid); err == nil {
			t.Fatal("invalid native local ID accepted")
		}
	}
}

func TestRDTHeaderFieldsAndAcknowledgements(t *testing.T) {
	data := rdtPacket{Type: rdtTypeData, Version: 5, SeqLow: 0x12345678, SeqHigh: 0x01020304,
		TxCount: 2, ConnID: 1, Aux: 0x8899, Payload: []byte{0xaa, 0xbb}}
	encoded, err := encodeRDTPacket(data)
	if err != nil || hex.EncodeToString(encoded) != "5a97c2f102050200785634120403020102019988aabb" {
		t.Fatal("native little-endian header layout mismatch")
	}
	decoded, err := decodeRDTPacket(encoded)
	if err != nil || decoded.SeqLow != data.SeqLow || decoded.SeqHigh != data.SeqHigh || decoded.TxCount != 2 || decoded.Aux != data.Aux || hex.EncodeToString(decoded.Payload) != "aabb" {
		t.Fatal("header fields did not decode")
	}
	for _, pair := range [][2]byte{{rdtTypeData, rdtTypeDataAck}, {rdtTypeFIN, rdtTypeFINAck}, {rdtTypeUrgentData, rdtTypeUrgentAck}, {rdtTypeAbort, rdtTypeAbortAck}} {
		data.Type = pair[0]
		ack, err := rdtAck(data, 7)
		if err != nil || ack.Type != pair[1] || ack.ConnID != 7 || ack.SeqLow != data.SeqLow || ack.Aux != data.Aux || ack.TxCount != data.TxCount || len(ack.Payload) != 0 {
			t.Fatal("native acknowledgement fields were not preserved")
		}
	}
	cack := rdtCumulativeAck(data, 8, 9, 7)
	if cack.Type != rdtTypeCumulativeAck || cack.SeqLow != 8 || cack.SeqHigh != 9 || cack.ConnID != 7 || cack.TxCount != data.TxCount || cack.Aux != data.Aux {
		t.Fatal("cumulative ACK fields differ from native reference")
	}
}

func TestRDTRejectsMalformedPacketBounds(t *testing.T) {
	packet, _ := rdtHello(1)
	frame, _ := encodeRDTPacket(packet)
	badMagic := append([]byte(nil), frame...)
	badMagic[0] = 0
	badVersion := append([]byte(nil), frame...)
	badVersion[5] = 4
	badLength := append([]byte(nil), frame...)
	badLength[6] = 1
	badType := append([]byte(nil), frame...)
	badType[4] = 0xff
	for _, bad := range [][]byte{nil, frame[:19], badMagic, badVersion, badLength, badType, append(frame, 0)} {
		if _, err := decodeRDTPacket(bad); err == nil {
			t.Fatal("malformed packet accepted")
		}
	}
	packet.Payload = make([]byte, rdtMaxPayload+1)
	if _, err := encodeRDTPacket(packet); err == nil {
		t.Fatal("oversized native-default packet accepted")
	}
	if _, err := rdtAck(rdtPacket{Type: rdtTypeCumulativeAck}, 1); err == nil {
		t.Fatal("ACK loop permitted")
	}
}

func TestRDTNativeSequenceBoundary(t *testing.T) {
	for _, test := range [][4]uint32{{0, 0, 1, 0}, {0xefffffff, 2, 0xf0000000, 2}, {0xf0000000, 2, 0, 3}} {
		low, high := rdtNextSequence(test[0], test[1])
		if low != test[2] || high != test[3] {
			t.Fatal("native low-word rollover mismatch")
		}
	}
}
