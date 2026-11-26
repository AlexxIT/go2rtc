package tutk

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

func DebugLog(format string, a ...any) {}

func Dial(host, uid, username, password string) (*Conn, error) {
	addr, err := net.ResolveUDPAddr("udp", host)
	if err != nil {
		// Default port for listening incoming LAN connections.
		// Important. It's not using for real connection.
		addr = &net.UDPAddr{IP: net.ParseIP(host), Port: 32761}
	}

	udpConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	c := &Conn{UDPConn: udpConn, addr: addr}

	sid := GenSessionID()

	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if addr.Port != 10001 {
		err = c.connectDirect(uid, sid)
	} else {
		err = c.connectRemote(uid, sid)
	}
	if err != nil {
		_ = c.Close()
		return nil, err
	}

	DebugLog("tutk: Dial connected, ver=%v, addr=%v", c.ver, c.addr)

	if c.ver[0] >= 25 {
		c.session = NewSession25(c, sid)
	} else {
		c.session = NewSession16(c, sid)
	}

	if err = c.clientStart(username, password); err != nil {
		DebugLog("tutk: clientStart failed: %v", err)
		_ = c.Close()
		return nil, err
	}
	DebugLog("tutk: clientStart succeeded!")

	go c.worker()

	return c, nil
}

type Conn struct {
	*net.UDPConn
	addr    *net.UDPAddr
	session Session

	ver    []byte
	err    error
	cmdMu  sync.Mutex
	cmdAck func()
}

// Read overwrite net.Conn
func (c *Conn) Read(buf []byte) (n int, err error) {
	for {
		var addr *net.UDPAddr
		if n, addr, err = c.UDPConn.ReadFromUDP(buf); err != nil {
			return 0, err
		}

		if string(c.addr.IP) != string(addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		if c.addr.Port != addr.Port {
			c.addr.Port = addr.Port
		}

		ReverseTransCodePartial(buf, buf[:n])
		//log.Printf("<- %x", buf[:n])
		return n, nil
	}
}

// Write overwrite net.Conn
func (c *Conn) Write(b []byte) (n int, err error) {
	//log.Printf("-> %x", b)
	return c.UDPConn.WriteToUDP(TransCodePartial(nil, b), c.addr)
}

// RemoteAddr overwrite net.Conn
func (c *Conn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *Conn) Protocol() string {
	return "tutk+udp"
}

func (c *Conn) Version() string {
	if len(c.ver) == 1 {
		return fmt.Sprintf("TUTK/%d", c.ver[0])
	}
	return fmt.Sprintf("TUTK/%d SDK %d.%d.%d.%d", c.ver[0], c.ver[1], c.ver[2], c.ver[3], c.ver[4])
}

func (c *Conn) ReadCommand() (ctrlType uint32, ctrlData []byte, err error) {
	return c.session.RecvIOCtrl()
}

func (c *Conn) WriteCommand(ctrlType uint32, ctrlData []byte) error {
	c.cmdMu.Lock()
	defer c.cmdMu.Unlock()

	DebugLog("tutk: WriteCommand 0x%04x len=%d data=%s", ctrlType, len(ctrlData), string(ctrlData))

	var repeat atomic.Int32
	repeat.Store(2)

	timeout := time.NewTicker(500 * time.Millisecond)
	defer timeout.Stop()

	c.cmdAck = func() {
		DebugLog("tutk: WriteCommand 0x%04x ACKed", ctrlType)
		repeat.Store(0)
		timeout.Reset(1)
	}

	buf := c.session.SendIOCtrl(ctrlType, ctrlData)

	for {
		if err := c.session.SessionWrite(0, buf); err != nil {
			return err
		}
		<-timeout.C
		r := repeat.Add(-1)
		if r < 0 {
			return nil
		}
		if r == 0 {
			DebugLog("tutk: WriteCommand 0x%04x NO ACK (continuing anyway)", ctrlType)
			return nil
		}
	}
}

func (c *Conn) ReadPacket() (hdr, payload []byte, err error) {
	return c.session.RecvFrameData()
}

func (c *Conn) WritePacket(hdr, payload []byte) error {
	buf := c.session.SendFrameData(hdr, payload)
	return c.session.SessionWrite(1, buf)
}

func (c *Conn) Error() error {
	if c.err != nil {
		return c.err
	}
	return io.EOF
}

func (c *Conn) worker() {
	DebugLog("tutk: worker started")
	defer func() {
		DebugLog("tutk: worker exiting, c.err=%v", c.err)
		c.session.Close()
	}()

	buf := make([]byte, 1200)

	for {
		n, err := c.Read(buf)
		if err != nil {
			c.err = fmt.Errorf("%s: %w", "tutk", err)
			DebugLog("tutk worker: Read error: %v", err)
			return
		}

		msgRes := c.handleMsg(buf[:n])
		if msgRes == msgUnknown {
			DebugLog("tutk: unknown msg[8]=0x%02x len=%d hex=%x", buf[8], n, buf[:min(32, n)])
		} else if msgRes == msgError {
			DebugLog("tutk: handleMsg returned msgError! packet=%x", buf[:min(32, n)])
		}

		switch msgRes {
		case msgCommandAck:
			DebugLog("tutk: got command ACK")
			if c.cmdAck != nil {
				c.cmdAck()
			}
		}
	}
}

const (
	msgUnknown = iota
	msgError
	msgPing
	msgUnknownPing
	msgClientStart
	msgClientStart2
	msgClientStartAck2
	msgCommand
	msgCommandAck
	msgCounters
	msgMediaChunk
	msgMediaFrame
	msgMediaReorder
	msgMediaLost
	msgCh5

	msgUnknown0007 // time sync without data?
	msgUnknown0008 // time sync with data?
	msgUnknown0010
	msgUnknown0013
	msgUnknown0900
	msgUnknown0a08
	msgUnknownCh1c
	msgDafang0012
)

func (c *Conn) handleMsg(msg []byte) int {
	// off sample
	// 0   0402      tutk magic
	// 2   120a      tutk version (120a, 190a...)
	// 4   0800      msg size = len(b)-16
	// 6   0000      channel seq
	// 8   28041200  msg type
	// 14  0100      channel (not all msg)
	// 28  0700      msg data (not all msg)
	switch msg[8] {
	case 0x08:
		switch ch := msg[14]; ch {
		case 0, 1:
			return c.session.SessionRead(ch, msg[28:])
		case 5:
			if len(msg) == 48 {
				_, _ = c.Write(msgAckCh5(msg))
				return msgCh5
			}
		case 0x1c:
			return msgUnknownCh1c
		}
	case 0x18:
		return msgUnknownPing
	case 0x28:
		if len(msg) == 24 {
			_, _ = c.Write(msgAckPing(msg))
			return msgPing
		}
	}
	return msgUnknown
}

func msgAckPing(msg []byte) []byte {
	// <- [24] 0402120a 08000000 28041200 000000005b0d4202070aa8c0
	// -> [24] 04021a0a 08000000 27042100 000000005b0d4202070aa8c0
	msg[8] = 0x27
	msg[10] = 0x21
	return msg
}

func msgAckCh5(msg []byte) []byte {
	// <- [48] 0402190a 20000400 07042100 7ecc05000c0000007ecc93c456c2561f 5a97c2f101050000000000000000000000010000
	// -> [48] 0402190a 20000400 08041200 7ecc05000c0000007ecc93c456c2561f 5a97c2f141050000000000000000000000010000
	msg[8] = 0x07
	msg[10] = 0x21
	msg[32] = 0x41
	return msg
}
