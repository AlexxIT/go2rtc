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
	s.sendMu.Lock()
	slot := &s.sendSlots[packetID%uidSendWindow]
	*slot = uidSendSlot{
		buffer: b, packet: packet, packetID: packetID,
		interval: uidRetransmit, used: true, writing: true,
	}
	s.sendMu.Unlock()
	err = s.writeDatagram(packet, true)
	s.sendMu.Lock()
	if err == nil {
		now := time.Now()
		slot.firstSend, slot.lastSend = now, now
	}
	slot.writing = false
	if err != nil || slot.acked {
		s.releaseSendSlot(slot)
	}
	s.sendMu.Unlock()
	if err != nil {
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
		deadline, changed, timeout := s.writeDeadline.snapshot()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, os.ErrDeadlineExceeded
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

// Caller holds sendMu. An in-flight datagram pins its slot and pooled bytes.
func (s *uidConn) releaseSendSlot(slot *uidSendSlot) {
	if slot.writing {
		slot.acked = true
		return
	}
	s.putBuffer(slot.buffer)
	*slot = uidSendSlot{}
	s.sendCount--
	s.signalSend()
}

func (s *uidConn) retransmitUID(now time.Time) error {
	var started time.Time
	current := now
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	for i := range s.sendSlots {
		slot := &s.sendSlots[i]
		if !slot.used || slot.writing || current.Sub(slot.lastSend) < slot.interval {
			continue
		}
		if current.Sub(slot.firstSend) >= s.timeout {
			return fmt.Errorf("baichuan: UID acknowledgement timed out")
		}
		slot.writing = true
		packet := slot.packet
		if started.IsZero() {
			started = time.Now()
		}
		s.sendMu.Unlock()
		err := s.writeDatagram(packet, false)
		// Preserve the caller's clock origin while accounting for blocked writes.
		current = now.Add(time.Since(started))
		s.sendMu.Lock()
		slot.writing = false
		if slot.acked {
			s.releaseSendSlot(slot)
		} else if err == nil {
			slot.lastSend = current
			slot.interval *= 2
			if slot.interval > uidMaxRetransmit {
				slot.interval = uidMaxRetransmit
			}
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return fmt.Errorf("baichuan: retransmit UID packet: %w", err)
		}
	}
	return nil
}
