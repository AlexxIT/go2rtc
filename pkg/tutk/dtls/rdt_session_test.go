package dtls

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

type fakeRDTDatagram struct {
	in, out                     chan []byte
	closed                      chan struct{}
	once                        sync.Once
	mu                          sync.Mutex
	readDeadline, writeDeadline time.Time
}

func newFakeRDTDatagram() *fakeRDTDatagram {
	return &fakeRDTDatagram{in: make(chan []byte, 256), out: make(chan []byte, 256), closed: make(chan struct{})}
}
func (f *fakeRDTDatagram) Read(p []byte) (int, error) {
	f.mu.Lock()
	deadline := f.readDeadline
	f.mu.Unlock()
	var timer *time.Timer
	var timeout <-chan time.Time
	if !deadline.IsZero() {
		timer = time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case b := <-f.in:
		if len(p) < len(b) {
			return copy(p, b), io.ErrShortBuffer
		}
		return copy(p, b), nil
	case <-f.closed:
		return 0, net.ErrClosed
	case <-timeout:
		return 0, osDeadlineError{}
	}
}
func (f *fakeRDTDatagram) Write(p []byte) (int, error) {
	f.mu.Lock()
	deadline := f.writeDeadline
	f.mu.Unlock()
	var timer *time.Timer
	var timeout <-chan time.Time
	if !deadline.IsZero() {
		timer = time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		timeout = timer.C
	}
	select {
	case f.out <- append([]byte(nil), p...):
		return len(p), nil
	case <-f.closed:
		return 0, net.ErrClosed
	case <-timeout:
		return 0, osDeadlineError{}
	}
}
func (f *fakeRDTDatagram) Close() error { f.once.Do(func() { close(f.closed) }); return nil }
func (f *fakeRDTDatagram) SetReadDeadline(t time.Time) error {
	f.mu.Lock()
	f.readDeadline = t
	f.mu.Unlock()
	return nil
}
func (f *fakeRDTDatagram) SetWriteDeadline(t time.Time) error {
	f.mu.Lock()
	f.writeDeadline = t
	f.mu.Unlock()
	return nil
}
func (f *fakeRDTDatagram) inject(p rdtPacket) {
	b, err := encodeRDTPacket(p)
	if err != nil {
		panic(err)
	}
	select {
	case f.in <- b:
	case <-f.closed:
	}
}
func shortRDTConfig() rdtSessionConfig {
	c := defaultRDTSessionConfig()
	c.handshake = 150 * time.Millisecond
	c.retry = 10 * time.Millisecond
	c.ioWait = 20 * time.Millisecond
	c.heartbeat = time.Hour
	c.shutdown = 60 * time.Millisecond
	c.attempts = 4
	return c
}

// Peer is deliberately scripted, not another copy of the session state machine.
func fakeRDTPeer(f *fakeRDTDatagram, handler func(rdtPacket) bool) {
	go func() {
		for {
			select {
			case <-f.closed:
				return
			case b := <-f.out:
				p, err := decodeRDTPacket(b)
				if err != nil {
					continue
				}
				if handler != nil && handler(p) {
					continue
				}
				switch p.Type {
				case rdtTypeHello:
					// ACK arriving before HELLO must not complete the symmetric handshake alone.
					ack, _ := rdtAck(p, 7)
					f.inject(ack)
					hello, _ := rdtHello(7)
					f.inject(hello)
				case rdtTypeData, rdtTypeFIN:
					ack, _ := rdtAck(p, 1)
					f.inject(ack)
				}
			}
		}
	}()
}
func newTestRDT(t *testing.T, handler func(rdtPacket) bool) (*rdtSession, *fakeRDTDatagram) {
	t.Helper()
	f := newFakeRDTDatagram()
	t.Cleanup(func() { _ = f.Close() })
	fakeRDTPeer(f, handler)
	s, err := newRDTSessionWithConfig(f, 1, shortRDTConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.stop(net.ErrClosed); <-s.stopped })
	return s, f
}
func TestRDTSessionRequiresSymmetricHandshake(t *testing.T) {
	f := newFakeRDTDatagram()
	defer f.Close()
	fakeRDTPeer(f, func(p rdtPacket) bool {
		if p.Type == rdtTypeHello {
			ack, _ := rdtAck(p, 7)
			f.inject(ack)
		}
		return true
	})
	c := shortRDTConfig()
	c.handshake = 45 * time.Millisecond
	started := time.Now()
	_, err := newRDTSessionWithConfig(f, 1, c)
	if err == nil || time.Since(started) > 300*time.Millisecond {
		t.Fatal("one-sided handshake succeeded or was unbounded")
	}
	select {
	case <-f.closed:
	default:
		t.Fatal("failed handshake leaked owned transport")
	}
}
func TestRDTSessionHelloRetryAndAckIdentity(t *testing.T) {
	var mu sync.Mutex
	hellos := 0
	s, _ := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeHello {
			return false
		}
		mu.Lock()
		defer mu.Unlock()
		hellos++
		return hellos == 1 // lost first HELLO, second completes normally
	})
	mu.Lock()
	count := hellos
	mu.Unlock()
	if count < 2 || s.peerID != 7 {
		t.Fatal("HELLO retry/peer identity not established")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestRDTSessionWriteRetrySplitAndForeignAck(t *testing.T) {
	var mu sync.Mutex
	var packets []rdtPacket
	s, f := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeData {
			return false
		}
		mu.Lock()
		packets = append(packets, p)
		count := len(packets)
		mu.Unlock()
		if count == 1 {
			// Foreign-ID and wrong/future sequence ACKs cannot accept this chunk.
			foreign := p
			foreign.Type = rdtTypeDataAck
			foreign.Payload = nil
			foreign.ConnID = 99
			fDummyInject := foreign
			_ = fDummyInject
			return true
		}
		return false
	})
	// The scripted hook above drops first ACK. Independently inject foreign ACKs.
	f.inject(rdtPacket{Type: rdtTypeDataAck, Version: 5, ConnID: 99})
	f.inject(rdtPacket{Type: rdtTypeCumulativeAck, Version: 5, ConnID: 1, SeqLow: 123})
	payload := bytes.Repeat([]byte{0x87}, rdtMaxPayload+9)
	n, err := s.Write(payload)
	if n != len(payload) || err != nil {
		t.Fatalf("write=%d,%v", n, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(packets) != 3 || packets[0].SeqLow != 0 || packets[1].SeqLow != 0 || packets[2].SeqLow != 1 || packets[0].TxCount != 1 || packets[1].TxCount != 2 || len(packets[2].Payload) != 9 || !bytes.Equal(packets[0].Payload, packets[1].Payload) {
		t.Fatal("retry altered bytes/sequence or chunk size")
	}
}
func TestRDTSessionReadReordersDeduplicatesAndAcknowledges(t *testing.T) {
	acks := make(chan rdtPacket, 20)
	s, f := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type == rdtTypeDataAck || p.Type == rdtTypeCumulativeAck {
			acks <- p
			return true
		}
		return false
	})
	p := rdtPacket{Type: rdtTypeData, Version: 5, ConnID: 1, SeqLow: 1, TxCount: 2, Aux: 0x1234, Payload: []byte("world")}
	f.inject(p)
	p.SeqLow = 0
	p.TxCount = 1
	p.Payload = []byte("hello")
	f.inject(p)
	p.SeqLow = 1
	p.Payload = []byte("world")
	f.inject(p)
	got := make([]byte, 10)
	if _, err := io.ReadFull(s, got); err != nil || string(got) != "helloworld" {
		t.Fatalf("read %q,%v", got, err)
	}
	deadline := time.After(200 * time.Millisecond)
	direct, cumulative := 0, false
	for direct < 3 || !cumulative {
		select {
		case ack := <-acks:
			if ack.ConnID != 7 {
				t.Fatal("ACK addressed wrong peer")
			}
			if ack.Type == rdtTypeDataAck {
				direct++
			}
			if ack.Type == rdtTypeCumulativeAck && ack.SeqLow == 1 {
				cumulative = true
				if ack.TxCount != 2 || ack.Aux != 0x1234 {
					t.Fatal("CACK did not retain first DATA auxiliary fields")
				}
			}
		case <-deadline:
			t.Fatal("accepted/duplicate DATA was not acknowledged")
		}
	}
	_ = s.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	_, err := s.Read(make([]byte, 1))
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatal("duplicate bytes leaked into byte stream")
	}
}
func TestRDTSessionReceiveBoundsDoNotAckDroppedData(t *testing.T) {
	acks := make(chan rdtPacket, 100)
	f := newFakeRDTDatagram()
	defer f.Close()
	fakeRDTPeer(f, func(p rdtPacket) bool {
		if p.Type == rdtTypeDataAck {
			acks <- p
			return true
		}
		return false
	})
	c := shortRDTConfig()
	c.buffered = rdtMaxPayload
	c.reorder = 2
	s, err := newRDTSessionWithConfig(f, 1, c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.stop(net.ErrClosed); <-s.stopped }()
	p := rdtPacket{Type: rdtTypeData, Version: 5, ConnID: 1, Payload: bytes.Repeat([]byte{1}, rdtMaxPayload)}
	f.inject(p)
	select {
	case <-acks:
	case <-time.After(time.Second):
		t.Fatal("initial packet not accepted")
	}
	p.SeqLow = 1
	p.Payload = []byte{2}
	f.inject(p)
	p.SeqLow = 20
	f.inject(p)
	select {
	case <-acks:
		t.Fatal("buffer/window dropped packet acknowledged")
	case <-time.After(20 * time.Millisecond):
	}
	got := make([]byte, rdtMaxPayload)
	if _, err = io.ReadFull(s, got); err != nil {
		t.Fatal(err)
	}
	p.SeqLow = 1
	f.inject(p)
	one := make([]byte, 1)
	if _, err = io.ReadFull(s, one); err != nil || one[0] != 2 {
		t.Fatal("retry after draining buffer was not accepted")
	}
}
func TestRDTSessionWriteFailureIsTerminal(t *testing.T) {
	s, _ := newTestRDT(t, func(p rdtPacket) bool { return p.Type == rdtTypeData })
	n, err := s.Write([]byte("uncertain"))
	if n != 0 || !errors.Is(err, errRDTRetry) {
		t.Fatalf("unacknowledged write=%d,%v", n, err)
	}
	if _, err = s.Write([]byte("next")); err == nil {
		t.Fatal("continued after ambiguous delivery")
	}
	if _, err = s.Read(make([]byte, 1)); err == nil {
		t.Fatal("read blocked after terminal failure")
	}
}
func TestRDTSessionFINCloseAndBufferedEOF(t *testing.T) {
	closes := make(chan rdtPacket, 4)
	s, f := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type == rdtTypeFIN || p.Type == rdtTypeClose {
			closes <- p
		}
		return false
	})
	f.inject(rdtPacket{Type: rdtTypeData, Version: 5, ConnID: 1, Payload: []byte("tail")})
	f.inject(rdtPacket{Type: rdtTypeFIN, Version: 5, ConnID: 1, SeqLow: 0xffffffff, SeqHigh: 0xffffffff})
	got := make([]byte, 4)
	if _, err := io.ReadFull(s, got); err != nil || string(got) != "tail" {
		t.Fatal("FIN lost buffered bytes")
	}
	if _, err := s.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("FIN read=%v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	fin := <-closes
	closePacket := <-closes
	if fin.Type != rdtTypeFIN || fin.SeqLow != 0xffffffff || fin.SeqHigh != 0xffffffff || closePacket.Type != rdtTypeClose {
		t.Fatal("native FIN/CLOSE order or sentinel differs")
	}
	if err := s.Close(); err != nil {
		t.Fatal("Close not idempotent")
	}
}
func TestRDTSessionCloseTimeoutAndWakeReader(t *testing.T) {
	s, _ := newTestRDT(t, func(p rdtPacket) bool { return p.Type == rdtTypeFIN })
	blocked := make(chan error, 1)
	go func() { _, err := s.Read(make([]byte, 1)); blocked <- err }()
	started := time.Now()
	err := s.Close()
	if err == nil || time.Since(started) > 300*time.Millisecond {
		t.Fatal("missing FIN ACK shutdown not bounded")
	}
	select {
	case err = <-blocked:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("blocked reader=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not wake reader")
	}
}

func TestRDTSessionPeerCloseCanFinishShutdown(t *testing.T) {
	var transport *fakeRDTDatagram
	s, f := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeFIN {
			return false
		}
		transport.inject(rdtPacket{Type: rdtTypeClose, Version: 5, ConnID: 0})
		return true
	})
	transport = f
	if err := s.Close(); err != nil {
		t.Fatalf("clean peer CLOSE reported as failure: %v", err)
	}
}
func TestRDTSessionFINWithGapIsNotCleanEOF(t *testing.T) {
	s, f := newTestRDT(t, nil)
	f.inject(rdtPacket{Type: rdtTypeData, Version: 5, ConnID: 1, SeqLow: 1, Payload: []byte("gap")})
	f.inject(rdtPacket{Type: rdtTypeFIN, Version: 5, ConnID: 1, SeqLow: 0xffffffff, SeqHigh: 0xffffffff})
	if _, err := s.Read(make([]byte, 1)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("gap silently truncated: %v", err)
	}
}

func TestRDTSessionCumulativeAckCanCompleteWrite(t *testing.T) {
	var f *fakeRDTDatagram
	s, transport := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeData {
			return false
		}
		ack := rdtCumulativeAck(p, p.SeqLow, p.SeqHigh, 1)
		f.inject(ack)
		return true
	})
	f = transport
	if n, err := s.Write([]byte("cumulative")); n != 10 || err != nil {
		t.Fatalf("CACK write=%d,%v", n, err)
	}
}

// Stock v3 firmware can send DATA and CACK with identifier zero even though
// HELLO negotiated nonzero IDs. The native receiver treats zero as unspecified
// on its already isolated SID/channel; a foreign nonzero ID is still rejected.
func TestRDTSessionZeroIdentifierDataDuringRequest(t *testing.T) {
	var transport *fakeRDTDatagram
	s, f := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeData {
			return false
		}
		transport.inject(rdtPacket{Type: rdtTypeData, Version: 5, ConnID: 99, Payload: []byte("foreign")})
		transport.inject(rdtPacket{Type: rdtTypeData, Version: 5, ConnID: 0, Payload: []byte("response")})
		transport.inject(rdtCumulativeAck(p, p.SeqLow, p.SeqHigh, 0))
		return true
	})
	transport = f
	_ = s.SetWriteDeadline(time.Now().Add(time.Second))
	_ = s.SetReadDeadline(time.Now().Add(time.Second))
	if n, err := s.Write([]byte("request")); n != 7 || err != nil {
		t.Fatalf("request=%d,%v", n, err)
	}
	got := make([]byte, 8)
	if _, err := io.ReadFull(s, got); err != nil || string(got) != "response" {
		t.Fatalf("response=%q,%v", got, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRDTSessionCloseDuringUnacknowledgedWrite(t *testing.T) {
	started := make(chan struct{}, 1)
	s, _ := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeData {
			return false
		}
		select {
		case started <- struct{}{}:
		default:
		}
		return true
	})
	written := make(chan error, 1)
	go func() { _, err := s.Write(make([]byte, 1024*1024)); written <- err }()
	<-started
	before := time.Now()
	_ = s.Close()
	if time.Since(before) > 300*time.Millisecond {
		t.Fatal("Close waited for entire multi-chunk Write")
	}
	select {
	case err := <-written:
		if err == nil {
			t.Fatal("cancelled Write reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("writer leaked on Close")
	}
}

func TestRDTSessionWrongFINAckCannotCompleteShutdown(t *testing.T) {
	var f *fakeRDTDatagram
	s, transport := newTestRDT(t, func(p rdtPacket) bool {
		if p.Type != rdtTypeFIN {
			return false
		}
		ack, _ := rdtAck(p, 1)
		ack.SeqHigh = 0 // Correct connection and packet type, wrong FIN sentinel.
		f.inject(ack)
		return true
	})
	f = transport
	if err := s.Close(); err == nil {
		t.Fatal("wrong-sequence FIN_ACK completed shutdown")
	}
}
