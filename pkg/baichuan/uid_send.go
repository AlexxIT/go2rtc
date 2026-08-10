package baichuan

import (
	"fmt"
	"net"
	"os"
	"time"
)

func (s *uidConn) writeChunk(payload []byte) error {
	packetID, err := s.reserveSend()
	if err != nil {
		return err
	}
	b := s.getBuffer()
	packet, err := encodeUIDData(b[:], s.cameraID, packetID, payload, uidMTU-uidDataHeader)
	if err != nil {
		s.putBuffer(b)
		s.cancelSendReservation()
		return err
	}
	now := time.Now()
	s.sendMu.Lock()
	slot := &s.sendSlots[packetID%uidSendWindow]
	*slot = uidSendSlot{
		buffer: b, packet: packet, packetID: packetID, firstSend: now, lastSend: now,
		interval: uidRetransmit, used: true,
	}
	s.sendMu.Unlock()
	if err = s.writeDatagram(packet); err != nil {
		s.releaseSend(packetID)
		select {
		case <-s.done:
			return s.writeError()
		default:
		}
		s.shutdown(err)
		return err
	}
	return nil
}

func (s *uidConn) cancelSendReservation() {
	s.sendMu.Lock()
	s.sendCount--
	s.signalSend()
	s.sendMu.Unlock()
}

func (s *uidConn) reserveSend() (uint32, error) {
	for {
		select {
		case <-s.done:
			return 0, s.writeError()
		default:
		}
		s.sendMu.Lock()
		if s.sendCount < uidSendWindow && !s.sendSlots[s.nextSend%uidSendWindow].used {
			packetID := s.nextSend
			s.nextSend++
			s.sendCount++
			s.sendMu.Unlock()
			return packetID, nil
		}
		s.sendMu.Unlock()

		deadline, changed, timeout := s.writeDeadline.snapshot()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, os.ErrDeadlineExceeded
		}
		select {
		case <-s.sendWake:
		case <-changed:
		case <-s.done:
			return 0, s.writeError()
		case <-timeout:
			return 0, os.ErrDeadlineExceeded
		}
	}
}

func (s *uidConn) releaseSend(packetID uint32) {
	s.sendMu.Lock()
	slot := &s.sendSlots[packetID%uidSendWindow]
	if slot.used && slot.packetID == packetID {
		s.putBuffer(slot.buffer)
		*slot = uidSendSlot{}
		s.sendCount--
		s.signalSend()
	}
	s.sendMu.Unlock()
}

func (s *uidConn) retransmitUID(now time.Time) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	for i := range s.sendSlots {
		slot := &s.sendSlots[i]
		if !slot.used || now.Sub(slot.lastSend) < slot.interval {
			continue
		}
		if now.Sub(slot.firstSend) >= s.timeout {
			return fmt.Errorf("baichuan: UID acknowledgement timed out")
		}
		if err := s.writeDatagram(slot.packet); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("baichuan: retransmit UID packet: %w", err)
		}
		slot.lastSend = now
		slot.interval *= 2
		if slot.interval > uidMaxRetransmit {
			slot.interval = uidMaxRetransmit
		}
	}
	return nil
}
