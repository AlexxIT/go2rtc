package baichuan

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"
)

const (
	uidSendWindow       = 64
	uidReceiveWindow    = 128
	uidReadQueue        = 512
	uidMaintenance      = 25 * time.Millisecond
	uidRetransmit       = 100 * time.Millisecond
	uidMaxRetransmit    = time.Second
	uidSocketReadBuffer = 1 << 20
)

var errUIDReadOverflow = errors.New("baichuan: UID read queue overflow")

type uidBuffer [uidMTU]byte

type uidChunk struct {
	buffer *uidBuffer
	data   []byte
}

type uidSendSlot struct {
	buffer    *uidBuffer
	packet    []byte
	packetID  uint32
	firstSend time.Time
	lastSend  time.Time
	interval  time.Duration
	used      bool
}

type uidReceiveSlot struct {
	chunk    uidChunk
	packetID uint32
	used     bool
}

type uidConn struct {
	conn     *net.UDPConn
	remote   netip.AddrPort
	clientID int32
	cameraID int32
	timeout  time.Duration

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
	errMu     sync.Mutex
	err       error

	readMu       sync.Mutex
	readCurrent  uidChunk
	readQueue    chan uidChunk
	readDeadline uidDeadline

	writeMu       sync.Mutex
	writeDeadline uidDeadline
	socketWriteMu sync.Mutex

	sendMu    sync.Mutex
	sendSlots [uidSendWindow]uidSendSlot
	sendCount int
	nextSend  uint32
	sendWake  chan struct{}

	receiveMu    sync.Mutex
	receiveSlots [uidReceiveWindow]uidReceiveSlot
	nextReceive  uint32
	received     bool
	ackDirty     bool

	pool sync.Pool
}

func newUIDConn(discovery *uidDiscovery, timeout time.Duration) (*uidConn, error) {
	if discovery == nil || discovery.conn == nil || !discovery.remote.IsValid() ||
		!discovery.remote.Addr().Is4() || discovery.remote.Port() == 0 ||
		discovery.clientID <= 0 || discovery.cameraID <= 0 || timeout <= 0 {
		return nil, errors.New("baichuan: invalid UID connection")
	}
	if err := discovery.conn.SetReadBuffer(uidSocketReadBuffer); err != nil {
		_ = discovery.conn.Close()
		return nil, fmt.Errorf("baichuan: set UID receive buffer: %w", err)
	}
	_ = discovery.conn.SetReadDeadline(time.Time{})
	s := &uidConn{
		conn: discovery.conn, remote: discovery.remote,
		clientID: discovery.clientID, cameraID: discovery.cameraID, timeout: timeout,
		done: make(chan struct{}), readQueue: make(chan uidChunk, uidReadQueue),
		sendWake: make(chan struct{}, 1),
	}
	s.readDeadline.init()
	s.writeDeadline.init()
	s.pool.New = func() any { return new(uidBuffer) }
	s.wg.Add(2)
	go s.readLoop()
	go s.maintenanceLoop()
	return s, nil
}

func (s *uidConn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	for {
		if len(s.readCurrent.data) != 0 {
			n := copy(p, s.readCurrent.data)
			s.readCurrent.data = s.readCurrent.data[n:]
			if len(s.readCurrent.data) == 0 {
				s.putBuffer(s.readCurrent.buffer)
				s.readCurrent = uidChunk{}
			}
			return n, nil
		}
		select {
		case <-s.done:
			return 0, s.readError()
		default:
		}
		select {
		case s.readCurrent = <-s.readQueue:
			continue
		default:
		}
		deadline, changed, timeout := s.readDeadline.snapshot()
		expired := !deadline.IsZero() && !time.Now().Before(deadline)
		if expired {
			return 0, os.ErrDeadlineExceeded
		}
		select {
		case chunk := <-s.readQueue:
			s.readCurrent = chunk
		case <-changed:
		case <-s.done:
			return 0, s.readError()
		case <-timeout:
			return 0, os.ErrDeadlineExceeded
		}
	}
}

func (s *uidConn) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	written := 0
	for len(p) != 0 {
		n := len(p)
		if n > uidMTU-uidDataHeader {
			n = uidMTU - uidDataHeader
		}
		if err := s.writeChunk(p[:n]); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}

func (s *uidConn) SetReadDeadline(value time.Time) error {
	s.readDeadline.set(value)
	return nil
}

func (s *uidConn) SetWriteDeadline(value time.Time) error {
	s.writeDeadline.set(value)
	// A later datagram installs a fresh bounded socket deadline. Keeping the
	// current one here prevents a concurrent clear from unbounding that write.
	if value.IsZero() {
		return nil
	}
	return s.conn.SetWriteDeadline(value)
}

func (s *uidConn) Close() error {
	s.shutdown(net.ErrClosed)
	s.readDeadline.set(time.Time{})
	s.writeDeadline.set(time.Time{})
	s.wg.Wait()
	s.writeMu.Lock()
	s.releaseBuffers()
	s.writeMu.Unlock()
	return nil
}

func (s *uidConn) shutdown(err error) {
	s.closeOnce.Do(func() {
		s.errMu.Lock()
		s.err = err
		s.errMu.Unlock()
		close(s.done)
		_ = s.conn.Close()
		s.signalSend()
	})
}

func (s *uidConn) readError() error {
	s.errMu.Lock()
	err := s.err
	s.errMu.Unlock()
	if errors.Is(err, net.ErrClosed) {
		return io.EOF
	}
	if err == nil {
		return io.EOF
	}
	return err
}

func (s *uidConn) writeError() error {
	if err := s.readError(); err != io.EOF {
		return err
	}
	return io.ErrClosedPipe
}

func (s *uidConn) getBuffer() *uidBuffer {
	return s.pool.Get().(*uidBuffer)
}

func (s *uidConn) putBuffer(b *uidBuffer) {
	if b != nil {
		s.pool.Put(b)
	}
}

func (s *uidConn) signalSend() {
	select {
	case s.sendWake <- struct{}{}:
	default:
	}
}

func (s *uidConn) writeDatagram(packet []byte) error {
	s.socketWriteMu.Lock()
	deadline := time.Now().Add(s.timeout)
	if value, _, _ := s.writeDeadline.snapshot(); !value.IsZero() && value.Before(deadline) {
		deadline = value
	}
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		s.socketWriteMu.Unlock()
		return err
	}
	_, err := s.conn.WriteToUDPAddrPort(packet, s.remote)
	s.socketWriteMu.Unlock()
	return err
}

func (s *uidConn) releaseBuffers() {
	s.readMu.Lock()
	if s.readCurrent.buffer != nil {
		s.putBuffer(s.readCurrent.buffer)
		s.readCurrent = uidChunk{}
	}
	for len(s.readQueue) != 0 {
		chunk := <-s.readQueue
		s.putBuffer(chunk.buffer)
	}
	s.readMu.Unlock()
	s.sendMu.Lock()
	for i := range s.sendSlots {
		if s.sendSlots[i].used {
			s.putBuffer(s.sendSlots[i].buffer)
			s.sendSlots[i] = uidSendSlot{}
		}
	}
	s.sendCount = 0
	s.sendMu.Unlock()
	s.receiveMu.Lock()
	for i := range s.receiveSlots {
		if s.receiveSlots[i].used {
			s.putBuffer(s.receiveSlots[i].chunk.buffer)
			s.receiveSlots[i] = uidReceiveSlot{}
		}
	}
	s.receiveMu.Unlock()
}
