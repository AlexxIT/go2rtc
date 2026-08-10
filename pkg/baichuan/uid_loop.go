package baichuan

import (
	"encoding/xml"
	"fmt"
	"time"
)

func (s *uidConn) readLoop() {
	defer s.wg.Done()
	for {
		b := s.getBuffer()
		n, remote, err := s.conn.ReadFromUDPAddrPort(b[:])
		if err != nil {
			s.putBuffer(b)
			select {
			case <-s.done:
				return
			default:
				s.shutdown(fmt.Errorf("baichuan: read UID transport: %w", err))
				return
			}
		}
		if remote != s.remote {
			s.putBuffer(b)
			continue
		}
		packet, err := parseUIDPacket(b[:n], uidMTU-uidDataHeader)
		if err != nil {
			s.putBuffer(b)
			continue
		}
		switch packet.magic {
		case uidMagicAck:
			s.handleUIDAck(packet)
			s.putBuffer(b)
		case uidMagicData:
			if packet.connectionID != s.clientID {
				s.putBuffer(b)
			} else if !s.handleUIDData(packet, b) {
				s.putBuffer(b)
			}
		case uidMagicDiscovery:
			s.handleUIDControl(packet)
			s.putBuffer(b)
		default:
			s.putBuffer(b)
		}
	}
}

func (s *uidConn) handleUIDAck(packet uidPacket) {
	if packet.connectionID != s.clientID || len(packet.payload) > uidSendWindow {
		return
	}
	s.sendMu.Lock()
	distance := uint32(s.nextSend - packet.packetID)
	if distance == 0 || distance > uidSendWindow {
		s.sendMu.Unlock()
		return
	}
	freed := false
	for i := range s.sendSlots {
		slot := &s.sendSlots[i]
		if !slot.used {
			continue
		}
		acked := int32(slot.packetID-packet.packetID) <= 0
		if !acked {
			distance := uint32(slot.packetID - packet.packetID - 1)
			acked = distance < uint32(len(packet.payload)) && packet.payload[distance] != 0
		}
		if acked {
			s.putBuffer(slot.buffer)
			*slot = uidSendSlot{}
			s.sendCount--
			freed = true
		}
	}
	if freed {
		s.signalSend()
	}
	s.sendMu.Unlock()
}

func (s *uidConn) handleUIDData(packet uidPacket, b *uidBuffer) bool {
	s.receiveMu.Lock()
	distance := int32(packet.packetID - s.nextReceive)
	if distance < 0 {
		s.ackDirty = s.received
		s.receiveMu.Unlock()
		return false
	}
	if distance >= uidReceiveWindow {
		s.ackDirty = s.received
		s.receiveMu.Unlock()
		return false
	}
	index := packet.packetID % uidReceiveWindow
	slot := &s.receiveSlots[index]
	if slot.used {
		s.ackDirty = s.received
		s.receiveMu.Unlock()
		return false
	}
	*slot = uidReceiveSlot{
		packetID: packet.packetID,
		chunk:    uidChunk{buffer: b, data: packet.payload},
		used:     true,
	}
	s.ackDirty = true
	for {
		slot = &s.receiveSlots[s.nextReceive%uidReceiveWindow]
		if !slot.used || slot.packetID != s.nextReceive {
			break
		}
		chunk := slot.chunk
		*slot = uidReceiveSlot{}
		s.nextReceive++
		s.received = true
		if len(chunk.data) == 0 {
			s.putBuffer(chunk.buffer)
			continue
		}
		select {
		case s.readQueue <- chunk:
		case <-s.done:
			s.putBuffer(chunk.buffer)
			s.receiveMu.Unlock()
			return true
		default:
			s.putBuffer(chunk.buffer)
			s.receiveMu.Unlock()
			s.shutdown(errUIDReadOverflow)
			return true
		}
	}
	s.receiveMu.Unlock()
	return true
}

func (s *uidConn) handleUIDControl(packet uidPacket) {
	xorUID(packet.payload, packet.payload, packet.transaction)
	var envelope struct {
		Disconnect *struct {
			CID int32 `xml:"cid"`
			DID int32 `xml:"did"`
		} `xml:"D2C_DISC"`
	}
	if xml.Unmarshal(packet.payload, &envelope) != nil {
		return
	}
	if envelope.Disconnect != nil &&
		envelope.Disconnect.CID == s.clientID && envelope.Disconnect.DID == s.cameraID {
		s.shutdown(fmt.Errorf("baichuan: camera closed UID transport"))
	}
}

func (s *uidConn) maintenanceLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(uidMaintenance)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case now := <-ticker.C:
			if err := s.retransmitUID(now); err != nil {
				s.shutdown(err)
				return
			}
			if err := s.writeUIDAck(); err != nil {
				s.shutdown(err)
				return
			}
		}
	}
}

func (s *uidConn) writeUIDAck() error {
	s.receiveMu.Lock()
	if !s.received || !s.ackDirty {
		s.receiveMu.Unlock()
		return nil
	}
	packetID := s.nextReceive - 1
	last := -1
	var payload [uidReceiveWindow]byte
	for distance := 0; distance < uidReceiveWindow; distance++ {
		id := s.nextReceive + uint32(distance)
		slot := &s.receiveSlots[id%uidReceiveWindow]
		if slot.used && slot.packetID == id {
			payload[distance] = 1
			last = distance
		}
	}
	s.ackDirty = false
	s.receiveMu.Unlock()

	b := s.getBuffer()
	packet, err := encodeUIDAck(b[:], s.cameraID, packetID, payload[:last+1], uidReceiveWindow)
	if err == nil {
		err = s.writeDatagram(packet)
	}
	s.putBuffer(b)
	if err != nil {
		return fmt.Errorf("baichuan: write UID acknowledgement: %w", err)
	}
	return nil
}
