package dtls

import (
	"encoding/binary"
	"errors"
)

// Native reference: official Wyze RDT 4.3.4.5, documented in
// RDT_EXPERIMENTAL.md. These helpers perform no I/O.
const (
	rdtMagic      = uint32(0xf1c2975a)
	rdtVersion    = byte(5)
	rdtHeaderSize = 20
	rdtMaxPayload = 1280 // Native library default; this probe does not negotiate changes.

	rdtTypeHello         = byte(0x01)
	rdtTypeData          = byte(0x02)
	rdtTypeFIN           = byte(0x03)
	rdtTypeClose         = byte(0x04)
	rdtTypeUrgentData    = byte(0x10)
	rdtTypeAbort         = byte(0x20)
	rdtTypeHelloAck      = byte(0x41)
	rdtTypeDataAck       = byte(0x42)
	rdtTypeFINAck        = byte(0x43)
	rdtTypeSACK          = byte(0x45)
	rdtTypeCumulativeAck = byte(0x46)
	rdtTypeUrgentAck     = byte(0x50)
	rdtTypeAbortAck      = byte(0x60)
	rdtTypeExit          = byte(0x70)
	rdtTypeExitAck       = byte(0x71)
)

type rdtPacket struct {
	Type    byte
	Version byte
	SeqLow  uint32
	SeqHigh uint32
	TxCount byte
	ConnID  byte
	Aux     uint16
	Payload []byte
}

func validRDTType(kind byte) bool {
	switch kind {
	case rdtTypeHello, rdtTypeData, rdtTypeFIN, rdtTypeClose, rdtTypeUrgentData,
		rdtTypeAbort, rdtTypeHelloAck, rdtTypeDataAck, rdtTypeFINAck, rdtTypeSACK,
		rdtTypeCumulativeAck, rdtTypeUrgentAck, rdtTypeAbortAck, rdtTypeExit, rdtTypeExitAck:
		return true
	}
	return false
}

func encodeRDTPacket(packet rdtPacket) ([]byte, error) {
	if packet.Version != rdtVersion || !validRDTType(packet.Type) || len(packet.Payload) > rdtMaxPayload {
		return nil, errors.New("unsupported RDT packet")
	}
	frame := make([]byte, rdtHeaderSize+len(packet.Payload))
	binary.LittleEndian.PutUint32(frame[0:], rdtMagic)
	frame[4], frame[5] = packet.Type, packet.Version
	binary.LittleEndian.PutUint16(frame[6:], uint16(len(packet.Payload)))
	binary.LittleEndian.PutUint32(frame[8:], packet.SeqLow)
	binary.LittleEndian.PutUint32(frame[12:], packet.SeqHigh)
	frame[16], frame[17] = packet.TxCount, packet.ConnID
	binary.LittleEndian.PutUint16(frame[18:], packet.Aux)
	copy(frame[20:], packet.Payload)
	return frame, nil
}

func decodeRDTPacket(frame []byte) (rdtPacket, error) {
	var packet rdtPacket
	if len(frame) < rdtHeaderSize || binary.LittleEndian.Uint32(frame) != rdtMagic {
		return packet, errors.New("invalid RDT header")
	}
	length := int(binary.LittleEndian.Uint16(frame[6:]))
	if frame[5] != rdtVersion || !validRDTType(frame[4]) || length > rdtMaxPayload || len(frame) != rdtHeaderSize+length {
		return packet, errors.New("unsupported or truncated RDT packet")
	}
	packet = rdtPacket{
		Type: frame[4], Version: frame[5],
		SeqLow: binary.LittleEndian.Uint32(frame[8:]), SeqHigh: binary.LittleEndian.Uint32(frame[12:]),
		TxCount: frame[16], ConnID: frame[17], Aux: binary.LittleEndian.Uint16(frame[18:]),
		Payload: append([]byte(nil), frame[20:]...),
	}
	return packet, nil
}

func rdtHello(localID byte) (rdtPacket, error) {
	if localID == 0 || localID == 255 {
		return rdtPacket{}, errors.New("RDT local ID must be between 1 and 254")
	}
	return rdtPacket{Type: rdtTypeHello, Version: rdtVersion, ConnID: localID}, nil
}

// rdtAck follows native FUN_0010aa34. HELLO_ACK echoes the incoming ID;
// DATA/FIN/urgent/abort acknowledgements instead address the learned peer ID.
func rdtAck(incoming rdtPacket, peerID byte) (rdtPacket, error) {
	ack := incoming
	ack.Version, ack.Payload = rdtVersion, nil
	switch incoming.Type {
	case rdtTypeHello:
		ack.Type = rdtTypeHelloAck
	case rdtTypeData:
		ack.Type, ack.ConnID = rdtTypeDataAck, peerID
	case rdtTypeFIN:
		ack.Type, ack.ConnID = rdtTypeFINAck, peerID
	case rdtTypeUrgentData:
		ack.Type, ack.ConnID = rdtTypeUrgentAck, peerID
	case rdtTypeAbort:
		ack.Type, ack.ConnID = rdtTypeAbortAck, peerID
	default:
		return rdtPacket{}, errors.New("RDT packet has no direct acknowledgement")
	}
	return ack, nil
}

// Native v5 CACKs name the highest contiguous received packet inclusively.
// reference is the receive state's retained first-DATA header, not necessarily
// the latest packet: the native code preserves its byte 16 and auxiliary bytes.
func rdtCumulativeAck(reference rdtPacket, seqLow, seqHigh uint32, peerID byte) rdtPacket {
	return rdtPacket{Type: rdtTypeCumulativeAck, Version: rdtVersion,
		SeqLow: seqLow, SeqHigh: seqHigh, TxCount: reference.TxCount, ConnID: peerID, Aux: reference.Aux}
}

// Native low-word rollover is at top nibble 0xf, not ordinary uint64 wrap.
func rdtNextSequence(low, high uint32) (uint32, uint32) {
	if low>>28 < 15 {
		return low + 1, high
	}
	return 0, high + 1
}
