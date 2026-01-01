package tutk

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func Dial(host, uid, model string) (*Conn, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		addr = &net.UDPAddr{IP: net.ParseIP(host), Port: 32761}
	}

	c := &Conn{conn: conn, addr: addr, sid: genSID()}

	if err = c.handshake([]byte(uid)); err != nil {
		_ = c.Close()
		return nil, err
	}

	switch model {
	case "isa.camera.df3":
		c.msgCtrl = c.oldMsgCtrl
		c.handleCh0 = c.oldHandlerCh0()
	default:
		c.msgCtrl = c.newMsgCtrl
		c.handleCh0 = c.newHandlerCh0()
	}

	c.rawCmd = make(chan []byte, 10)
	c.rawPkt = make(chan []byte, 100)

	go c.worker()

	return c, nil
}

type Conn struct {
	conn *net.UDPConn
	addr *net.UDPAddr
	sid  []byte

	err    error
	rawCmd chan []byte
	rawPkt chan []byte

	cmdMu  sync.Mutex
	cmdAck func()

	seqSendCh0 uint16
	seqSendCh1 uint16

	seqSendCmd1 uint16
	seqSendCmd2 uint16
	seqSendCnt  uint16

	seqRecvPkt0 uint16
	seqRecvPkt1 uint16
	seqRecvCmd2 uint16

	msgCtrl   func(ctrlType uint16, ctrlData []byte) []byte
	handleCh0 func(cmd []byte) int8
}

func (c *Conn) handshake(uid []byte) (err error) {
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err = c.WriteAndWait(
		c.msgConnectByUID(uid, 1),
		func(_, res []byte) bool {
			return bytes.Index(res, uid) == 16 // 02061200
		},
	); err != nil {
		return err
	}

	if err = c.Write(c.msgConnectByUID(uid, 2)); err != nil {
		return err
	}

	if _, err = c.WriteAndWait(
		c.msgAvClientStart(),
		func(req, res []byte) bool {
			return bytes.Index(res, req[48:52]) == 48
		},
	); err != nil {
		return err
	}

	_ = c.SetDeadline(time.Time{})

	return nil
}

func (c *Conn) worker() {
	defer func() {
		close(c.rawCmd)
		close(c.rawPkt)
	}()

	buf := make([]byte, 1200)

	for {
		n, _, err := c.ReadFromUDP(buf)
		if err != nil {
			c.err = fmt.Errorf("%s: %w", "tutk", err)
			return
		}

		if c.handleMsg(buf[:n]) <= 0 {
			if c.err != nil {
				return
			}
			fmt.Printf("tutk: unknown msg: %x\n", buf[:n])
		}
	}
}

func (c *Conn) Write(buf []byte) error {
	//log.Printf("-> %x", buf)
	_, err := c.conn.WriteToUDP(TransCodePartial(nil, buf), c.addr)
	return err
}

func (c *Conn) ReadFromUDP(buf []byte) (n int, addr *net.UDPAddr, err error) {
	for {
		if n, addr, err = c.conn.ReadFromUDP(buf); err != nil {
			return 0, nil, err
		}

		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		ReverseTransCodePartial(buf, buf[:n])
		//log.Printf("<- %x", buf[:n])
		return n, addr, nil
	}
}

func (c *Conn) WriteAndWait(req []byte, ok func(req, res []byte) bool) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if err := c.Write(req); err == nil && t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	buf := make([]byte, 1200)

	for {
		n, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if ok(req, buf[:n]) {
			c.addr.Port = addr.Port
			return buf[:n], nil
		}
	}
}

func (c *Conn) Protocol() string {
	return "tutk+udp"
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
	buf, ok := <-c.rawCmd
	if !ok {
		return 0, nil, c.Error()
	}
	cmd = binary.LittleEndian.Uint16(buf[:2])
	data = buf[4:]
	return
}

// WriteCommand will send a command every second five times
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

	msg := c.msgCtrl(cmd, data)

	for {
		if err := c.WriteCh0(msg); err != nil {
			return err
		}
		<-timeout.C
		r := repeat.Add(-1)
		if r < 0 {
			return nil
		}
		if r == 0 {
			return fmt.Errorf("%s: can't send command %d", "tutk", cmd)
		}
	}
}

func (c *Conn) ReadPacket() ([]byte, error) {
	buf, ok := <-c.rawPkt
	if !ok {
		return nil, c.Error()
	}
	return buf, nil
}

func (c *Conn) WritePacket(data []byte) error {
	panic("not implemented")
}

func genSID() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b[8:])
	copy(b, b[8:10])
	b[4] = 0x0c
	return b
}
