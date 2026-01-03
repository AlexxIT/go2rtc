package cs2

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func Dial(host, transport string) (*Conn, error) {
	conn, err := handshake(host, transport)
	if err != nil {
		return nil, err
	}

	_, isTCP := conn.(*tcpConn)

	c := &Conn{
		conn:   conn,
		isTCP:  isTCP,
		rawCh0: make(chan []byte, 10),
		rawCh2: make(chan []byte, 100),
	}
	go c.worker()
	return c, nil
}

type Conn struct {
	conn  net.Conn
	isTCP bool

	err    error
	seqCh0 uint16
	seqCh3 uint16
	rawCh0 chan []byte
	rawCh2 chan []byte

	cmdMu  sync.Mutex
	cmdAck func()
}

const (
	magic        = 0xF1
	magicDrw     = 0xD1
	msgLanSearch = 0x30
	msgPunchPkt  = 0x41
	msgP2PRdyUDP = 0x42
	msgP2PRdyTCP = 0x43
	msgDrw       = 0xD0
	msgDrwAck    = 0xD1
	msgPing      = 0xE0
	msgPong      = 0xE1
	msgClose     = 0xF1
)

func handshake(host, transport string) (net.Conn, error) {
	conn, err := newUDPConn(host, 32108)
	if err != nil {
		return nil, err
	}

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := []byte{magic, msgLanSearch, 0, 0}
	res, err := conn.(*udpConn).WriteUntil(req, func(res []byte) bool {
		return res[1] == msgPunchPkt
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	var msgUDP, msgTCP byte

	if transport == "" || transport == "udp" {
		msgUDP = msgP2PRdyUDP
	}
	if transport == "" || transport == "tcp" {
		msgTCP = msgP2PRdyTCP
	}

	res, err = conn.(*udpConn).WriteUntil(res, func(res []byte) bool {
		return res[1] == msgUDP || res[1] == msgTCP
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	_ = conn.SetDeadline(time.Time{})

	if res[1] == msgTCP {
		_ = conn.Close()
		//host := fmt.Sprintf("%d.%d.%d.%d:%d", b[31], b[30], b[29], b[28], uint16(b[27])<<8|uint16(b[26]))
		return newTCPConn(conn.RemoteAddr().String())
	}

	return conn, nil
}

func (c *Conn) worker() {
	defer func() {
		close(c.rawCh0)
		close(c.rawCh2)
	}()

	chAck := make([]uint16, 4) // only for UDP
	buf := make([]byte, 1200)
	var ch2WaitSize int
	var ch2WaitData []byte
	var keepaliveTS time.Time

	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			c.err = fmt.Errorf("%s: %w", "cs2", err)
			return
		}

		switch buf[1] {
		case msgDrw:
			ch := buf[5]

			if c.isTCP {
				// For TCP we should using ping/pong.
				if now := time.Now(); now.After(keepaliveTS) {
					_, _ = c.conn.Write([]byte{magic, msgPing, 0, 0})
					keepaliveTS = now.Add(5 * time.Second)
				}
			} else {
				// For UDP we should using ack.
				seqHI := buf[6]
				seqLO := buf[7]

				if chAck[ch] != uint16(seqHI)<<8|uint16(seqLO) {
					continue
				}
				chAck[ch]++

				ack := []byte{magic, msgDrwAck, 0, 6, magicDrw, ch, 0, 1, seqHI, seqLO}
				_, _ = c.conn.Write(ack)
			}

			switch ch {
			case 0:
				select {
				case c.rawCh0 <- buf[12:]:
				default:
				}
				continue

			case 2:
				ch2WaitData = append(ch2WaitData, buf[8:n]...)

				for len(ch2WaitData) > 4 {
					if ch2WaitSize == 0 {
						ch2WaitSize = int(binary.BigEndian.Uint32(ch2WaitData))
						ch2WaitData = ch2WaitData[4:]
					}
					if ch2WaitSize <= len(ch2WaitData) {
						select {
						case c.rawCh2 <- ch2WaitData[:ch2WaitSize]:
						default:
							c.err = fmt.Errorf("%s: media queue is full", "cs2")
							return
						}

						ch2WaitData = ch2WaitData[ch2WaitSize:]
						ch2WaitSize = 0
					} else {
						break
					}
				}
				continue
			}

		case msgPing:
			_, _ = c.conn.Write([]byte{magic, msgPong, 0, 0})
			continue
		case msgPong, msgP2PRdyUDP, msgP2PRdyTCP, msgClose:
			continue // skip it
		case msgDrwAck: // only for UDP
			if c.cmdAck != nil {
				c.cmdAck()
			}
			continue
		}

		fmt.Printf("%s: unknown msg: %x\n", "cs2", buf[:n])
	}
}

func (c *Conn) Protocol() string {
	if c.isTCP {
		return "cs2+tcp"
	}
	return "cs2+udp"
}

func (c *Conn) Version() string {
	return "CS2"
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) Error() error {
	if c.err != nil {
		return c.err
	}
	return io.EOF
}

func (c *Conn) ReadCommand() (cmd uint32, data []byte, err error) {
	buf, ok := <-c.rawCh0
	if !ok {
		return 0, nil, c.Error()
	}
	return binary.LittleEndian.Uint32(buf), buf[4:], nil
}

func (c *Conn) WriteCommand(cmd uint32, data []byte) error {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()

	req := marshalCmd(0, c.seqCh0, cmd, data)
	c.seqCh0++

	if c.isTCP {
		_, err := c.conn.Write(req)
		return err
	}

	var repeat atomic.Int32
	repeat.Store(5)

	timeout := time.NewTicker(time.Second)
	defer timeout.Stop()

	c.cmdAck = func() {
		repeat.Store(0)
		timeout.Reset(1)
	}

	for {
		if _, err := c.conn.Write(req); err != nil {
			return err
		}
		<-timeout.C
		r := repeat.Add(-1)
		if r < 0 {
			return nil
		}
		if r == 0 {
			return fmt.Errorf("%s: can't send command %d", "cs2", cmd)
		}
	}
}

func (c *Conn) ReadPacket() (hdr, payload []byte, err error) {
	data, ok := <-c.rawCh2
	if !ok {
		return nil, nil, c.Error()
	}
	return data[:32], data[32:], nil
}

func (c *Conn) WritePacket(hdr, payload []byte) error {
	const offset = 12

	n := 32 + uint32(len(payload))
	req := make([]byte, n+offset)
	req[0] = magic
	req[1] = msgDrw
	binary.BigEndian.PutUint16(req[2:], uint16(n+8))

	req[4] = magicDrw
	req[5] = 3 // channel
	binary.BigEndian.PutUint16(req[6:], c.seqCh3)
	c.seqCh3++
	binary.BigEndian.PutUint32(req[8:], n)
	copy(req[offset:], hdr)
	copy(req[offset+32:], payload)

	_, err := c.conn.Write(req)
	return err
}

func marshalCmd(channel byte, seq uint16, cmd uint32, payload []byte) []byte {
	size := len(payload)
	req := make([]byte, 4+4+4+4+size)

	// 1. message header (4 bytes)
	req[0] = magic
	req[1] = msgDrw
	binary.BigEndian.PutUint16(req[2:], uint16(4+4+4+size))

	// 2. drw? header (4 bytes)
	req[4] = magicDrw
	req[5] = channel
	binary.BigEndian.PutUint16(req[6:], seq)

	// 3. payload size (4 bytes)
	binary.BigEndian.PutUint32(req[8:], uint32(4+size))

	// 4. payload command (4 bytes)
	binary.BigEndian.PutUint32(req[12:], cmd)

	// 5. payload
	copy(req[16:], payload)

	return req
}

func newUDPConn(host string, port int) (net.Conn, error) {
	// We using raw net.UDPConn, because RemoteAddr should be changed during handshake.
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		addr = &net.UDPAddr{IP: net.ParseIP(host), Port: port}
	}

	return &udpConn{UDPConn: conn, addr: addr}, nil
}

type udpConn struct {
	*net.UDPConn
	addr *net.UDPAddr
}

func (c *udpConn) Read(p []byte) (n int, err error) {
	var addr *net.UDPAddr
	for {
		n, addr, err = c.UDPConn.ReadFromUDP(p)
		if err != nil {
			return 0, err
		}

		if string(addr.IP) == string(c.addr.IP) || n >= 8 {
			return
		}
	}
}

func (c *udpConn) Write(req []byte) (n int, err error) {
	//log.Printf("-> %x", req)
	return c.UDPConn.WriteToUDP(req, c.addr)
}

func (c *udpConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *udpConn) WriteUntil(req []byte, ok func(res []byte) bool) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if _, err := c.Write(req); err == nil && t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	buf := make([]byte, 1200)

	for {
		n, addr, err := c.UDPConn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		if ok(buf[:n]) {
			c.addr.Port = addr.Port
			return buf[:n], nil
		}
	}
}

func newTCPConn(addr string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	return &tcpConn{conn.(*net.TCPConn), bufio.NewReader(conn)}, nil
}

type tcpConn struct {
	*net.TCPConn
	rd *bufio.Reader
}

func (c *tcpConn) Read(p []byte) (n int, err error) {
	tmp := make([]byte, 8)
	if _, err = io.ReadFull(c.rd, tmp); err != nil {
		return
	}
	n = int(binary.BigEndian.Uint16(tmp))
	if len(p) < n {
		return 0, fmt.Errorf("tcp: buffer too small")
	}
	_, err = io.ReadFull(c.rd, p[:n])
	//log.Printf("<- %x%x", tmp, p[:n])
	return
}

func (c *tcpConn) Write(req []byte) (n int, err error) {
	n = len(req)
	buf := make([]byte, 8+n)
	binary.BigEndian.PutUint16(buf, uint16(n))
	buf[2] = 0x68
	copy(buf[8:], req)
	//log.Printf("-> %x", buf)
	_, err = c.TCPConn.Write(buf)
	return
}
