package baichuan

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"sort"
)

const (
	udpMagicDiscovery = 0x2A87CF3A
	udpMagicAck       = 0x2A87CF20
	udpMagicData      = 0x2A87CF10
)

type udpPacket interface {
	packetType() uint32
}

type udpDiscoveryPacket struct {
	TID     uint32
	Payload []byte
}

func (udpDiscoveryPacket) packetType() uint32 { return udpMagicDiscovery }

type udpAckPacket struct {
	ConnectionID int32
	PacketID     uint32
	Payload      []byte
}

func (udpAckPacket) packetType() uint32 { return udpMagicAck }

type udpDataPacket struct {
	ConnectionID int32
	PacketID     uint32
	Payload      []byte
}

func (udpDataPacket) packetType() uint32 { return udpMagicData }

type udpP2PEnvelope struct {
	XMLName xml.Name       `xml:"P2P"`
	C2DC    *udpC2DC       `xml:"C2D_C,omitempty"`
	D2CCR   *udpD2CCR      `xml:"D2C_C_R,omitempty"`
	C2DDisc *udpDisconnect `xml:"C2D_DISC,omitempty"`
	D2CDisc *udpDisconnect `xml:"D2C_DISC,omitempty"`
}

type udpC2DC struct {
	UID    string      `xml:"uid"`
	Client udpPortList `xml:"cli"`
	CID    int32       `xml:"cid"`
	MTU    uint32      `xml:"mtu"`
	Debug  int         `xml:"debug"`
	OS     string      `xml:"p"`
}

type udpPortList struct {
	Port int `xml:"port"`
}

type udpD2CCR struct {
	CID int32 `xml:"cid"`
	DID int32 `xml:"did"`
}

type udpDisconnect struct {
	CID int32 `xml:"cid"`
	DID int32 `xml:"did"`
}

func marshalUDPXML(v any) ([]byte, error) {
	body, err := xml.Marshal(v)
	if err != nil {
		return nil, err
	}

	return append([]byte(xml.Header), body...), nil
}

func marshalUDPPacket(packet udpPacket) ([]byte, error) {
	switch pkt := packet.(type) {
	case udpDiscoveryPacket:
		checksum := udpCRC32(pkt.Payload)
		buf := make([]byte, 20+len(pkt.Payload))
		binary.LittleEndian.PutUint32(buf[0:4], udpMagicDiscovery)
		binary.LittleEndian.PutUint32(buf[4:8], uint32(len(pkt.Payload))) //#nosec G115
		binary.LittleEndian.PutUint32(buf[8:12], 1)
		binary.LittleEndian.PutUint32(buf[12:16], pkt.TID)
		binary.LittleEndian.PutUint32(buf[16:20], checksum)
		copy(buf[20:], pkt.Payload)
		return buf, nil

	case udpAckPacket:
		buf := make([]byte, 28+len(pkt.Payload))
		binary.LittleEndian.PutUint32(buf[0:4], udpMagicAck)
		binary.LittleEndian.PutUint32(buf[4:8], uint32(pkt.ConnectionID)) //#nosec G115
		binary.LittleEndian.PutUint32(buf[8:12], 0)
		binary.LittleEndian.PutUint32(buf[12:16], 0)
		binary.LittleEndian.PutUint32(buf[16:20], pkt.PacketID)
		binary.LittleEndian.PutUint32(buf[20:24], 0)
		binary.LittleEndian.PutUint32(buf[24:28], uint32(len(pkt.Payload))) //#nosec G115
		copy(buf[28:], pkt.Payload)
		return buf, nil

	case udpDataPacket:
		buf := make([]byte, 20+len(pkt.Payload))
		binary.LittleEndian.PutUint32(buf[0:4], udpMagicData)
		binary.LittleEndian.PutUint32(buf[4:8], uint32(pkt.ConnectionID)) //#nosec G115
		binary.LittleEndian.PutUint32(buf[8:12], 0)
		binary.LittleEndian.PutUint32(buf[12:16], pkt.PacketID)
		binary.LittleEndian.PutUint32(buf[16:20], uint32(len(pkt.Payload))) //#nosec G115
		copy(buf[20:], pkt.Payload)
		return buf, nil
	default:
		return nil, fmt.Errorf("unsupported udp packet %T", packet)
	}
}

func parseUDPPacket(buf []byte) (udpPacket, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("short udp packet")
	}

	switch binary.LittleEndian.Uint32(buf[0:4]) {
	case udpMagicDiscovery:
		if len(buf) < 20 {
			return nil, fmt.Errorf("short udp discovery packet")
		}
		payloadLen := binary.LittleEndian.Uint32(buf[4:8])
		tid := binary.LittleEndian.Uint32(buf[12:16])
		checksum := binary.LittleEndian.Uint32(buf[16:20])
		if len(buf) < 20+int(payloadLen) {
			return nil, fmt.Errorf("short udp discovery payload")
		}
		payload := append([]byte(nil), buf[20:20+payloadLen]...)
		if udpCRC32(payload) != checksum {
			return nil, fmt.Errorf("udp discovery checksum mismatch")
		}
		return udpDiscoveryPacket{
			TID:     tid,
			Payload: payload,
		}, nil

	case udpMagicAck:
		if len(buf) < 28 {
			return nil, fmt.Errorf("short udp ack packet")
		}
		payloadLen := binary.LittleEndian.Uint32(buf[24:28])
		if len(buf) < 28+int(payloadLen) {
			return nil, fmt.Errorf("short udp ack payload")
		}
		return udpAckPacket{
			ConnectionID: int32(binary.LittleEndian.Uint32(buf[4:8])), //#nosec G115
			PacketID:     binary.LittleEndian.Uint32(buf[16:20]),
			Payload:      append([]byte(nil), buf[28:28+payloadLen]...),
		}, nil

	case udpMagicData:
		if len(buf) < 20 {
			return nil, fmt.Errorf("short udp data packet")
		}
		payloadLen := binary.LittleEndian.Uint32(buf[16:20])
		if len(buf) < 20+int(payloadLen) {
			return nil, fmt.Errorf("short udp data payload")
		}
		return udpDataPacket{
			ConnectionID: int32(binary.LittleEndian.Uint32(buf[4:8])), //#nosec G115
			PacketID:     binary.LittleEndian.Uint32(buf[12:16]),
			Payload:      append([]byte(nil), buf[20:20+payloadLen]...),
		}, nil

	default:
		return nil, fmt.Errorf("unknown udp packet magic %#x", binary.LittleEndian.Uint32(buf[0:4]))
	}
}

func udpCRC32(buf []byte) uint32 {
	const poly = 0xEDB88320

	var crc uint32
	for _, b := range buf {
		crc ^= uint32(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

func contiguousPayloads(store map[uint32][]byte, consumed *uint32, hasConsumed *bool) [][]byte {
	results := make([][]byte, 0)
	next := uint32(0)
	if *hasConsumed {
		next = *consumed + 1
	}

	for {
		payload, ok := store[next]
		if !ok {
			break
		}
		delete(store, next)
		results = append(results, payload)
		*consumed = next
		*hasConsumed = true
		next++
	}

	return results
}

func ackWindow(store map[uint32][]byte, consumed uint32, hasConsumed bool) (uint32, []byte, bool) {
	if !hasConsumed {
		return 0, nil, false
	}

	start := consumed
	for {
		if _, ok := store[start+1]; !ok {
			break
		}
		start++
	}

	keys := make([]uint32, 0, len(store))
	for k := range store {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return start, nil, true
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	end := keys[len(keys)-1]

	payload := make([]byte, 0, end-start)
	for id := start + 1; id <= end; id++ {
		if _, ok := store[id]; ok {
			payload = append(payload, 1)
		} else {
			payload = append(payload, 0)
		}
	}

	return start, payload, true
}
