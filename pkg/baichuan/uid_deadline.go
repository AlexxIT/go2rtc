package baichuan

import (
	"os"
	"sync"
	"time"
)

func (s *uidConn) SetWriteDeadline(value time.Time) error {
	s.socketDeadlineMu.Lock()
	defer s.socketDeadlineMu.Unlock()
	s.writeDeadline.set(value)
	if s.socketDeadline.IsZero() || !s.socketApplication {
		return nil
	}
	return s.setSocketWriteDeadline()
}

func (s *uidConn) writeDatagram(packet []byte, application bool) error {
	if err := s.acquireSocketWriter(application); err != nil {
		return err
	}
	defer func() {
		s.socketDeadlineMu.Lock()
		s.socketDeadline = time.Time{}
		s.socketDeadlineMu.Unlock()
		<-s.socketWriter
	}()
	if err := s.beginDatagram(application); err != nil {
		return err
	}
	_, err := s.conn.WriteToUDPAddrPort(packet, s.remote)
	return err
}

func (s *uidConn) acquireSocketWriter(application bool) error {
	select {
	case s.socketWriter <- struct{}{}:
		return nil
	default:
	}
	if !application {
		select {
		case s.socketWriter <- struct{}{}:
			return nil
		case <-s.done:
			return s.writeError()
		}
	}
	for {
		deadline, changed, timeout := s.writeDeadline.snapshot()
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return os.ErrDeadlineExceeded
		}
		select {
		case s.socketWriter <- struct{}{}:
			return nil
		case <-changed:
		case <-timeout:
			return os.ErrDeadlineExceeded
		case <-s.done:
			return s.writeError()
		}
	}
}

// socketWriter owns the datagram; socketDeadlineMu lets deadline updates
// interrupt application writes without waiting for socket I/O or altering ACKs.
func (s *uidConn) beginDatagram(application bool) error {
	s.socketDeadlineMu.Lock()
	defer s.socketDeadlineMu.Unlock()
	s.socketApplication = application
	s.socketDeadline = time.Now().Add(s.timeout)
	return s.setSocketWriteDeadline()
}

// Caller holds socketDeadlineMu. Clearing an application deadline restores the
// active datagram's transport bound rather than making that write unbounded.
func (s *uidConn) setSocketWriteDeadline() error {
	deadline := s.socketDeadline
	if s.socketApplication {
		if value, _, _ := s.writeDeadline.snapshot(); !value.IsZero() && value.Before(deadline) {
			deadline = value
		}
	}
	return s.conn.SetWriteDeadline(deadline)
}

type uidDeadline struct {
	mu      sync.Mutex
	value   time.Time
	changed chan struct{}
	timer   *time.Timer
}

func (d *uidDeadline) set(value time.Time) {
	d.mu.Lock()
	if !d.timer.Stop() {
		select {
		case <-d.timer.C:
		default:
		}
	}
	close(d.changed)
	d.value = value
	d.changed = make(chan struct{})
	if !value.IsZero() {
		duration := time.Until(value)
		if duration < 0 {
			duration = 0
		}
		d.timer.Reset(duration)
	}
	d.mu.Unlock()
}

func (d *uidDeadline) init() {
	d.changed = make(chan struct{})
	d.timer = time.NewTimer(time.Hour)
	d.timer.Stop()
}

func (d *uidDeadline) snapshot() (time.Time, <-chan struct{}, <-chan time.Time) {
	d.mu.Lock()
	value, changed := d.value, d.changed
	var timeout <-chan time.Time
	if !value.IsZero() {
		timeout = d.timer.C
	}
	d.mu.Unlock()
	return value, changed, timeout
}
