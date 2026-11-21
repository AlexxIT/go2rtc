package baichuan

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	uidMagicDiscovery uint32 = 0x2A87CF3A
	uidMagicAck       uint32 = 0x2A87CF20
	uidMagicData      uint32 = 0x2A87CF10

	uidDiscoveryHeader = 20
	uidAckHeader       = 28
	uidDataHeader      = 20
	uidMaxDatagram     = 65507
)

var uidXMLKey = [...]uint32{
	0x1f2d3c4b, 0x5a6c7f8d, 0x38172e4b, 0x8271635a,
	0x863f1a2b, 0xa5c6f7d8, 0x8371e1b4, 0x17f2d3a5,
}

type uidPacket struct {
	magic        uint32
	connectionID int32
	packetID     uint32
	transaction  uint32
	payload      []byte
}

func marshalUIDDiscovery(transaction uint32, payload []byte) ([]byte, error) {
	if err := checkUIDPayload(len(payload), uidDiscoveryHeader, uidMaxDatagram-uidDiscoveryHeader); err != nil {
		return nil, err
	}
	b := make([]byte, uidDiscoveryHeader+len(payload))
	binary.LittleEndian.PutUint32(b, uidMagicDiscovery)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(payload)))
	binary.LittleEndian.PutUint32(b[8:], 1)
	binary.LittleEndian.PutUint32(b[12:], transaction)
	binary.LittleEndian.PutUint32(b[16:], uidChecksum(payload))
	copy(b[uidDiscoveryHeader:], payload)
	return b, nil
}

func marshalUIDData(connectionID int32, packetID uint32, payload []byte, maxPayload int) ([]byte, error) {
	b := make([]byte, uidDataHeader+len(payload))
	return encodeUIDData(b, connectionID, packetID, payload, maxPayload)
}

func encodeUIDData(b []byte, connectionID int32, packetID uint32, payload []byte, maxPayload int) ([]byte, error) {
	if err := checkUIDPayload(len(payload), uidDataHeader, maxPayload); err != nil {
		return nil, err
	}
	if len(b) < uidDataHeader+len(payload) {
		return nil, fmt.Errorf("baichuan: short UID data buffer")
	}
	b = b[:uidDataHeader+len(payload)]
	clear(b[:uidDataHeader])
	binary.LittleEndian.PutUint32(b, uidMagicData)
	binary.LittleEndian.PutUint32(b[4:], uint32(connectionID))
	binary.LittleEndian.PutUint32(b[12:], packetID)
	binary.LittleEndian.PutUint32(b[16:], uint32(len(payload)))
	copy(b[uidDataHeader:], payload)
	return b, nil
}

func marshalUIDAck(connectionID int32, packetID uint32, payload []byte, maxPayload int) ([]byte, error) {
	b := make([]byte, uidAckHeader+len(payload))
	return encodeUIDAck(b, connectionID, packetID, payload, maxPayload)
}

func encodeUIDAck(b []byte, connectionID int32, packetID uint32, payload []byte, maxPayload int) ([]byte, error) {
	if err := checkUIDPayload(len(payload), uidAckHeader, maxPayload); err != nil {
		return nil, err
	}
	if len(b) < uidAckHeader+len(payload) {
		return nil, fmt.Errorf("baichuan: short UID ACK buffer")
	}
	b = b[:uidAckHeader+len(payload)]
	clear(b[:uidAckHeader])
	binary.LittleEndian.PutUint32(b, uidMagicAck)
	binary.LittleEndian.PutUint32(b[4:], uint32(connectionID))
	binary.LittleEndian.PutUint32(b[16:], packetID)
	binary.LittleEndian.PutUint32(b[24:], uint32(len(payload)))
	copy(b[uidAckHeader:], payload)
	return b, nil
}

func parseUIDPacket(b []byte, maxPayload int) (uidPacket, error) {
	if len(b) < 4 {
		return uidPacket{}, fmt.Errorf("baichuan: short UID packet")
	}
	if len(b) > uidMaxDatagram || maxPayload < 0 {
		return uidPacket{}, fmt.Errorf("baichuan: UID packet exceeds limit")
	}
	p := uidPacket{magic: binary.LittleEndian.Uint32(b)}
	var header, sizeAt int
	switch p.magic {
	case uidMagicDiscovery:
		header, sizeAt = uidDiscoveryHeader, 4
		if len(b) >= header {
			p.transaction = binary.LittleEndian.Uint32(b[12:])
		}
	case uidMagicAck:
		header, sizeAt = uidAckHeader, 24
		if len(b) >= header {
			p.connectionID = int32(binary.LittleEndian.Uint32(b[4:]))
			p.packetID = binary.LittleEndian.Uint32(b[16:])
		}
	case uidMagicData:
		header, sizeAt = uidDataHeader, 16
		if len(b) >= header {
			p.connectionID = int32(binary.LittleEndian.Uint32(b[4:]))
			p.packetID = binary.LittleEndian.Uint32(b[12:])
		}
	default:
		return uidPacket{}, fmt.Errorf("baichuan: unknown UID packet")
	}
	if len(b) < header {
		return uidPacket{}, fmt.Errorf("baichuan: short UID packet")
	}
	size := binary.LittleEndian.Uint32(b[sizeAt:])
	if uint64(size) > uint64(maxPayload) || uint64(size) != uint64(len(b)-header) {
		return uidPacket{}, fmt.Errorf("baichuan: invalid UID payload length")
	}
	p.payload = b[header:]
	if p.magic == uidMagicDiscovery && uidChecksum(p.payload) != binary.LittleEndian.Uint32(b[16:]) {
		return uidPacket{}, fmt.Errorf("baichuan: UID checksum mismatch")
	}
	return p, nil
}

func checkUIDPayload(size, header, maxPayload int) error {
	if size < 0 || maxPayload < 0 || size > maxPayload || size > uidMaxDatagram-header {
		return fmt.Errorf("baichuan: UID payload exceeds limit")
	}
	return nil
}

func xorUID(dst, src []byte, transaction uint32) {
	dst = dst[:len(src)]
	for i, value := range src {
		key := uidXMLKey[(i>>2)%len(uidXMLKey)] + transaction
		dst[i] = value ^ byte(key>>((i&3)*8))
	}
}

func uidChecksum(b []byte) uint32 {
	table := crc32.IEEETable
	var sum uint32
	for _, value := range b {
		sum = table[byte(sum)^value] ^ sum>>8
	}
	return sum
}
