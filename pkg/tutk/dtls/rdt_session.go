package dtls

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// rdtDatagram is a decrypted, message-preserving channel (e.g. pion dtls.Conn).
// A session owns this channel, including Close; it must not be shared with any
// other reader or deadline owner. No camera, auth, or file commands live here.
type rdtDatagram interface {
	io.Reader
	io.Writer
	io.Closer
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

type rdtSessionConfig struct {
	handshake, retry, ioWait, heartbeat, shutdown time.Duration
	attempts, reorder, buffered                   int
}

func defaultRDTSessionConfig() rdtSessionConfig {
	return rdtSessionConfig{5 * time.Second, 200 * time.Millisecond, 250 * time.Millisecond,
		time.Second, time.Second, 6, 64, 256 * 1024}
}

// This is deliberately a bounded stop-and-wait sender, not a replacement for
// every vendor RDT feature. Provenance: RDT_EXPERIMENTAL.md,
// native FUN_00108734/00108b74 (symmetric HELLO), FUN_001094bc/0010aa34
// (receive/ACK), FUN_00105c48/001084f8 (DATA), RDT_Destroy (FIN then CLOSE).
// Selective ACKs and urgent writes are not implemented.
type rdtSession struct {
	conn                           rdtDatagram
	cfg                            rdtSessionConfig
	localID                        byte
	mu                             sync.Mutex
	peerID                         byte
	helloSeen, helloAckSeen        bool
	recvNext, sendNext             uint64
	firstData                      *rdtPacket
	pending                        map[uint64][]byte
	pendingBytes                   int
	received                       bytes.Buffer
	eof, closing                   bool
	err                            error
	readDeadline, writeDeadline    time.Time
	notify                         chan struct{}
	ack                            chan rdtPacket
	ready                          chan struct{}
	done                           chan struct{}
	stopped                        chan struct{}
	writeMu, wireMu                sync.Mutex
	readyOnce, stopOnce, closeOnce sync.Once
	closeErr                       error
}

var errRDTRetry = errors.New("RDT acknowledgement timeout")

func newRDTSession(conn rdtDatagram, localID byte) (*rdtSession, error) {
	return newRDTSessionWithConfig(conn, localID, defaultRDTSessionConfig())
}

func newRDTSessionWithConfig(conn rdtDatagram, localID byte, cfg rdtSessionConfig) (*rdtSession, error) {
	if _, err := rdtHello(localID); err != nil {
		return nil, err
	}
	if conn == nil || cfg.handshake <= 0 || cfg.handshake > 5*time.Second || cfg.retry <= 0 || cfg.ioWait <= 0 || cfg.heartbeat <= 0 || cfg.shutdown <= 0 || cfg.attempts < 1 || cfg.attempts > 20 || cfg.reorder < 1 || cfg.reorder > 120 || cfg.buffered < rdtMaxPayload {
		return nil, errors.New("invalid RDT session configuration")
	}
	s := &rdtSession{conn: conn, cfg: cfg, localID: localID, pending: make(map[uint64][]byte),
		notify: make(chan struct{}, 1), ack: make(chan rdtPacket, 32), ready: make(chan struct{}), done: make(chan struct{}), stopped: make(chan struct{})}
	go s.receive()
	timer := time.NewTimer(cfg.handshake)
	defer timer.Stop()
	ticker := time.NewTicker(cfg.retry)
	defer ticker.Stop()
	hello, _ := rdtHello(localID)
	for {
		if err := s.send(hello); err != nil {
			s.stop(err)
			<-s.stopped
			return nil, err
		}
		select {
		case <-s.ready:
			go s.keepalive()
			return s, nil
		case <-s.done:
			<-s.stopped
			return nil, s.failure()
		case <-timer.C:
			s.stop(errors.New("RDT handshake timeout"))
			<-s.stopped
			return nil, s.failure()
		case <-ticker.C:
		}
	}
}

func (s *rdtSession) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}
func (s *rdtSession) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	return net.ErrClosed
}
func (s *rdtSession) stop(err error) {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		if s.err == nil {
			s.err = err
		}
		s.mu.Unlock()
		close(s.done)
		_ = s.conn.Close() // Unblocks the sole transport reader.
		s.signal()
	})
}
func (s *rdtSession) send(packet rdtPacket) error {
	frame, err := encodeRDTPacket(packet)
	if err != nil {
		return err
	}
	s.wireMu.Lock()
	defer s.wireMu.Unlock()
	select {
	case <-s.done:
		return s.failure()
	default:
	}
	if err = s.conn.SetWriteDeadline(time.Now().Add(s.cfg.ioWait)); err != nil {
		return errors.New("RDT write deadline failed")
	}
	n, err := s.conn.Write(frame)
	if err != nil {
		return errors.New("RDT transport write failed")
	}
	if n != len(frame) {
		return io.ErrShortWrite
	}
	return nil
}
func (s *rdtSession) keepalive() {
	t := time.NewTicker(s.cfg.heartbeat)
	defer t.Stop()
	hello, _ := rdtHello(s.localID)
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.mu.Lock()
			closing := s.closing
			s.mu.Unlock()
			if closing {
				continue
			}
			if err := s.send(hello); err != nil {
				s.stop(err)
				return
			}
		}
	}
}

func (s *rdtSession) receive() {
	defer close(s.stopped)
	frame := make([]byte, rdtHeaderSize+rdtMaxPayload+1)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		if err := s.conn.SetReadDeadline(time.Now().Add(s.cfg.ioWait)); err != nil {
			s.stop(errors.New("RDT read deadline failed"))
			return
		}
		n, err := s.conn.Read(frame)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
			}
			var timeout net.Error
			if errors.As(err, &timeout) && timeout.Timeout() {
				continue
			}
			s.stop(errors.New("RDT transport read failed"))
			return
		}
		p, err := decodeRDTPacket(frame[:n])
		if err != nil {
			continue
		}
		if err = s.accept(p); err != nil {
			s.stop(err)
			return
		}
	}
}

func (s *rdtSession) accept(p rdtPacket) error {
	s.mu.Lock()
	if p.Type == rdtTypeHello {
		if len(p.Payload) != 0 || p.ConnID == 0 || p.ConnID == 255 || (s.helloSeen && p.ConnID != s.peerID) {
			s.mu.Unlock()
			return nil
		}
		s.peerID = p.ConnID
		s.helloSeen = true
	} else if p.ConnID != 0 && p.ConnID != s.localID {
		// The native RDT receiver treats zero as unspecified on an already
		// authenticated SID/channel. Stock v3 firmware uses it for DATA/CACK.
		s.mu.Unlock()
		return nil
	}
	if p.Type == rdtTypeHelloAck && len(p.Payload) == 0 {
		s.helloAckSeen = true
	}
	ready := s.helloSeen && s.helloAckSeen
	peer := s.peerID
	s.mu.Unlock()
	if p.Type == rdtTypeHello {
		ack, _ := rdtAck(p, peer)
		if err := s.send(ack); err != nil {
			return err
		}
		if ready {
			s.readyOnce.Do(func() { close(s.ready) })
		}
		return nil
	}
	if ready {
		s.readyOnce.Do(func() { close(s.ready) })
	}
	if !ready {
		return nil
	}
	switch p.Type {
	case rdtTypeData:
		return s.acceptData(p, peer)
	case rdtTypeDataAck, rdtTypeCumulativeAck, rdtTypeFINAck:
		if len(p.Payload) == 0 {
			select {
			case s.ack <- p:
			default:
			}
		}
	case rdtTypeFIN:
		if len(p.Payload) != 0 {
			return nil
		}
		ack, _ := rdtAck(p, peer)
		if err := s.send(ack); err != nil {
			return err
		}
		s.mu.Lock()
		s.eof = true
		if len(s.pending) != 0 && s.err == nil {
			s.err = io.ErrUnexpectedEOF
		}
		s.mu.Unlock()
		s.signal()
	case rdtTypeClose:
		s.mu.Lock()
		s.eof = true
		if len(s.pending) != 0 && s.err == nil {
			s.err = io.ErrUnexpectedEOF
		}
		s.mu.Unlock()
		s.signal()
		s.stop(io.EOF)
	case rdtTypeAbort:
		ack, _ := rdtAck(p, peer)
		_ = s.send(ack)
		return errors.New("RDT peer aborted")
	case rdtTypeUrgentData:
		return errors.New("RDT urgent stream unsupported")
	}
	return nil
}

// Native sequence space includes low word f0000000 before rollover to high+1.
func rdtOrdinal(low, high uint32) (uint64, bool) {
	if low > 0xf0000000 {
		return 0, false
	}
	return uint64(high)*0xf0000001 + uint64(low), true
}
func rdtSequence(ordinal uint64) (uint32, uint32) {
	return uint32(ordinal % 0xf0000001), uint32(ordinal / 0xf0000001)
}

func (s *rdtSession) acceptData(p rdtPacket, peer byte) error {
	seq, ok := rdtOrdinal(p.SeqLow, p.SeqHigh)
	if !ok || len(p.Payload) == 0 {
		return nil
	}
	s.mu.Lock()
	if s.eof {
		s.mu.Unlock()
		return nil
	}
	accepted := seq < s.recvNext
	if seq >= s.recvNext && seq-s.recvNext < uint64(s.cfg.reorder) {
		if _, exists := s.pending[seq]; exists {
			accepted = true
		} else if s.received.Len()+s.pendingBytes+len(p.Payload) <= s.cfg.buffered {
			s.pending[seq] = p.Payload
			s.pendingBytes += len(p.Payload)
			accepted = true
			if s.firstData == nil {
				first := p
				first.Payload = nil
				s.firstData = &first
			}
			for {
				payload, exists := s.pending[s.recvNext]
				if !exists {
					break
				}
				_, _ = s.received.Write(payload)
				delete(s.pending, s.recvNext)
				s.pendingBytes -= len(payload)
				s.recvNext++
			}
		}
	}
	next := s.recvNext
	var ref rdtPacket
	if s.firstData != nil {
		ref = *s.firstData
	}
	s.mu.Unlock()
	if !accepted {
		return nil
	} // Never acknowledge bytes we could not retain.
	ack, _ := rdtAck(p, peer)
	if err := s.send(ack); err != nil {
		return err
	}
	if next > 0 {
		low, high := rdtSequence(next - 1)
		if err := s.send(rdtCumulativeAck(ref, low, high, peer)); err != nil {
			return err
		}
	}
	s.signal()
	return nil
}

func (s *rdtSession) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		s.mu.Lock()
		if s.received.Len() > 0 {
			n, _ := s.received.Read(p)
			s.mu.Unlock()
			return n, nil
		}
		err, eof, closing, deadline := s.err, s.eof, s.closing, s.readDeadline
		s.mu.Unlock()
		if err != nil {
			return 0, err
		}
		if eof {
			return 0, io.EOF
		}
		if closing {
			return 0, net.ErrClosed
		}
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return 0, osDeadlineError{}
		}
		var timer *time.Timer
		var timeout <-chan time.Time
		if !deadline.IsZero() {
			timer = time.NewTimer(time.Until(deadline))
			timeout = timer.C
		}
		select {
		case <-s.notify:
		case <-s.done:
		case <-timeout:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

// io.Writer reports only acknowledged chunks. If a chunk's acknowledgement is
// lost through all retries, its delivery is uncertain: the session is terminal
// so callers cannot accidentally continue at an ambiguous byte-stream offset.
func (s *rdtSession) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	closing, err := s.closing, s.err
	s.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if closing {
		return 0, net.ErrClosed
	}
	n := 0
	for len(p) > 0 {
		s.mu.Lock()
		closing := s.closing
		s.mu.Unlock()
		if closing {
			return n, net.ErrClosed
		}
		length := min(len(p), rdtMaxPayload)
		low, high := rdtSequence(s.sendNext)
		s.mu.Lock()
		peer := s.peerID
		deadline := s.writeDeadline
		s.mu.Unlock()
		packet := rdtPacket{Type: rdtTypeData, Version: rdtVersion, SeqLow: low, SeqHigh: high, ConnID: peer, Payload: append([]byte(nil), p[:length]...)}
		if err := s.sendReliable(packet, deadline, s.cfg.attempts); err != nil {
			s.stop(err)
			return n, err
		}
		s.sendNext++
		n += length
		p = p[length:]
	}
	return n, nil
}
func (s *rdtSession) sendReliable(packet rdtPacket, deadline time.Time, attempts int) error {
	for attempt := 0; attempt < attempts; attempt++ {
		if !deadline.IsZero() && !time.Now().Before(deadline) {
			return osDeadlineError{}
		}
		packet.TxCount = byte(attempt + 1)
		if err := s.send(packet); err != nil {
			return err
		}
		wait := s.cfg.retry
		if !deadline.IsZero() && time.Until(deadline) < wait {
			wait = time.Until(deadline)
		}
		timer := time.NewTimer(wait)
		acknowledged := false
	waitAck:
		for {
			select {
			case <-s.done:
				timer.Stop()
				return s.failure()
			case <-timer.C:
				break waitAck
			case ack := <-s.ack:
				if packet.Type == rdtTypeFIN {
					acknowledged = ack.Type == rdtTypeFINAck && ack.SeqLow == packet.SeqLow && ack.SeqHigh == packet.SeqHigh
				} else if ack.Type == rdtTypeDataAck {
					acknowledged = ack.SeqLow == packet.SeqLow && ack.SeqHigh == packet.SeqHigh
				} else if ack.Type == rdtTypeCumulativeAck {
					a, aok := rdtOrdinal(ack.SeqLow, ack.SeqHigh)
					b, bok := rdtOrdinal(packet.SeqLow, packet.SeqHigh)
					// Stop-and-wait never has a legitimate future DATA ACK.
					acknowledged = aok && bok && a == b
				}
				if acknowledged {
					timer.Stop()
					return nil
				}
			}
		}
	}
	return errRDTRetry
}
func (s *rdtSession) SetReadDeadline(t time.Time) error {
	s.mu.Lock()
	s.readDeadline = t
	s.mu.Unlock()
	s.signal()
	return nil
}
func (s *rdtSession) SetWriteDeadline(t time.Time) error {
	s.mu.Lock()
	s.writeDeadline = t
	s.mu.Unlock()
	return nil
}

// Close prevents new chunks and wakes readers, then acquires the bounded
// sender. Native Destroy flushes already-written data, sends FIN with both words
// ffffffff, then CLOSE. There are no unflushed user bytes in this sender.
func (s *rdtSession) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closing = true
		s.mu.Unlock()
		s.signal()
		// An in-flight chunk finishes its bounded retry loop before FIN.
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
		s.mu.Lock()
		peer, prior := s.peerID, s.err
		s.mu.Unlock()
		if prior == nil {
			fin := rdtPacket{Type: rdtTypeFIN, Version: rdtVersion, SeqLow: 0xffffffff, SeqHigh: 0xffffffff, ConnID: peer}
			s.closeErr = s.sendReliable(fin, time.Now().Add(s.cfg.shutdown), s.cfg.attempts)
			// Native peers may finish with CLOSE instead of acknowledging our FIN.
			// receive records gaps as UnexpectedEOF, so only clean EOF is benign.
			if errors.Is(s.closeErr, io.EOF) {
				s.closeErr = nil
			}
			_ = s.send(rdtPacket{Type: rdtTypeClose, Version: rdtVersion, ConnID: peer})
		}
		s.stop(net.ErrClosed)
		<-s.stopped
	})
	return s.closeErr
}

type osDeadlineError struct{}

func (osDeadlineError) Error() string   { return "RDT operation deadline exceeded" }
func (osDeadlineError) Timeout() bool   { return true }
func (osDeadlineError) Temporary() bool { return true }
