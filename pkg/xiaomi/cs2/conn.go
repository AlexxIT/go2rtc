package cs2

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func Dial(host string) (*Conn, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		conn: conn,
		addr: &net.UDPAddr{IP: net.ParseIP(host), Port: 32108},
	}

	if err = c.handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}

	c.rawCh0 = make(chan []byte, 10)
	c.rawCh2 = make(chan []byte, 100)

	go c.worker()

	return c, nil
}

type Conn struct {
	conn *net.UDPConn
	addr *net.UDPAddr

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
	msgP2PRdy    = 0x42
	msgDrw       = 0xD0
	msgDrwAck    = 0xD1
	msgAlive     = 0xE0
)

func (c *Conn) handshake() error {
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	buf, err := c.WriteAndWait([]byte{magic, msgLanSearch, 0, 0}, msgPunchPkt)
	if err != nil {
		return fmt.Errorf("%s: read punch: %w", "cs2", err)
	}

	_, err = c.WriteAndWait(buf, msgP2PRdy)
	if err != nil {
		return fmt.Errorf("%s: read ready: %w", "cs2", err)
	}

	_ = c.Write([]byte{magic, msgAlive, 0, 0})

	_ = c.SetDeadline(time.Time{})

	return nil
}

func (c *Conn) worker() {
	defer func() {
		close(c.rawCh0)
		close(c.rawCh2)
	}()

	chAck := make([]uint16, 4)
	buf := make([]byte, 1200)
	var ch2WaitSize int
	var ch2WaitData []byte

	for {
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			c.err = fmt.Errorf("%s: %w", "cs2", err)
			return
		}

		if string(addr.IP) != string(c.addr.IP) || n < 8 || buf[0] != magic {
			continue // skip messages from another IP
		}

		//log.Printf("<- %x", buf[:n])

		switch buf[1] {
		case msgDrw:
			ch := buf[5]
			seqHI := buf[6]
			seqLO := buf[7]

			if chAck[ch] != uint16(seqHI)<<8|uint16(seqLO) {
				continue
			}
			chAck[ch]++

			ack := []byte{magic, msgDrwAck, 0, 6, magicDrw, ch, 0, 1, seqHI, seqLO}
			if _, err = c.conn.WriteToUDP(ack, c.addr); err != nil {
				return
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

		case msgP2PRdy: // skip it
			continue
		case msgDrwAck:
			if c.cmdAck != nil {
				c.cmdAck()
			}
			continue
		}

		fmt.Printf("%s: unknown msg: %x\n", "cs2", buf[:n])
	}
}

func (c *Conn) Write(req []byte) error {
	//log.Printf("-> %x", req)
	_, err := c.conn.WriteToUDP(req, c.addr)
	return err
}

func (c *Conn) WriteAndWait(req []byte, waitMsg uint8) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if err := c.Write(req); err == nil && t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	buf := make([]byte, 1200)

	for {
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		if buf[0] == magic && buf[1] == waitMsg {
			c.addr.Port = addr.Port
			return buf[:n], nil
		}
	}
}

func (c *Conn) RemoteAddr() net.Addr {
	return c.addr
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

func (c *Conn) ReadCommand() (cmd uint16, data []byte, err error) {
	buf, ok := <-c.rawCh0
	if !ok {
		return 0, nil, c.Error()
	}
	cmd = binary.LittleEndian.Uint16(buf[:2])
	data = buf[4:]
	return
}

func (c *Conn) WriteCommand(cmd uint16, data []byte) error {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()

	var repeat atomic.Int32
	repeat.Store(5)

	timeout := time.NewTicker(time.Second)
	defer timeout.Stop()

	c.cmdAck = func() {
		repeat.Store(0)
		timeout.Reset(1)
	}

	req := marshalCmd(0, c.seqCh0, uint32(cmd), data)
	c.seqCh0++

	for {
		if err := c.Write(req); err != nil {
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

func (c *Conn) ReadPacket() ([]byte, error) {
	data, ok := <-c.rawCh2
	if !ok {
		return nil, c.Error()
	}
	return data, nil
}

func (c *Conn) WritePacket(data []byte) error {
	const offset = 12

	n := uint32(len(data))
	req := make([]byte, n+offset)
	req[0] = magic
	req[1] = msgDrw
	binary.BigEndian.PutUint16(req[2:], uint16(n+8))

	req[4] = magicDrw
	req[5] = 3 // channel
	binary.BigEndian.PutUint16(req[6:], c.seqCh3)
	c.seqCh3++
	binary.BigEndian.PutUint32(req[8:], n)
	copy(req[offset:], data)

	return c.Write(req)
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
