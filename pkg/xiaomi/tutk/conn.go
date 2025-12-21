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

func Dial(host, uid string) (*Conn, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	c := &Conn{
		conn: conn,
		addr: &net.UDPAddr{IP: net.ParseIP(host), Port: 32761},
		sid:  genSID(),
	}

	if err = c.handshake([]byte(uid)); err != nil {
		_ = c.Close()
		return nil, err
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
	seqCh0 uint16
	seqCmd uint16
	rawCmd chan []byte
	rawPkt chan []byte

	cmdMu  sync.Mutex
	cmdAck func()
}

func (c *Conn) handshake(uid []byte) (err error) {
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err = c.WriteAndWait(
		c.msgLanSearch(uid, 1), // 01062100
		func(_, res []byte) bool {
			return bytes.Index(res, uid) == 16 // 02061200
		},
	); err != nil {
		return err
	}

	if err = c.Write(c.msgLanSearch(uid, 2)); err != nil {
		return err
	}

	if _, err = c.WriteAndWait(
		c.msgAvClientStartReq(), // 07042100 + 00000b00
		func(req, res []byte) bool {
			mid := req[48:52]
			return bytes.Index(res, mid) == 48 // 08041200 + 00140800
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
	var waitSeq uint16
	var waitSize uint32
	var waitData []byte

	for {
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			c.err = fmt.Errorf("%s: %w", "tutk", err)
			return
		}

		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		b := ReverseTransCodePartial(buf[:n])
		//log.Printf("<- %x", b)

		if b[0] != 0x04 || b[1] != 0x02 {
			continue
		}

		if len(b) == 24 {
			_ = c.Write(msgAckPing(b))
			continue
		}

		switch b[14] {
		case 0:
			switch string(b[28:30]) {
			case "\x00\x12":
				_ = c.Write(c.msgAckCh0Req0012(b))
				continue

			case "\x00\x70":
				_ = c.Write(c.msgAckCh0Req0070(b))
				select {
				case c.rawCmd <- b[52:]:
				default:
				}
				continue

			case "\x00\x71":
				if c.cmdAck != nil {
					c.cmdAck()
				}
				continue

			case "\x01\x03":
				seq := binary.LittleEndian.Uint16(b[40:])
				if seq != waitSeq {
					waitSeq = 0 // data loss
					continue
				}
				if seq == 0 {
					waitSize = binary.LittleEndian.Uint32(b[36:]) + 32
				}

				waitData = append(waitData, b[52:]...)
				if n := uint32(len(waitData)); n < waitSize {
					waitSeq++
					continue
				} else if n > waitSize {
					waitSeq = 0 // data loss
					continue
				}

				// create a buffer for the header and collected data
				packetData := make([]byte, waitSize)
				// there's a header at the end - let's move it to the beginning
				copy(packetData, waitData[waitSize-32:])
				copy(packetData[32:], waitData)

				select {
				case c.rawPkt <- packetData:
				default:
					c.err = fmt.Errorf("%s: media queue is full", "tutk")
					return
				}

				waitSeq = 0
				waitData = waitData[:0]
				continue

			case "\x01\x04":
				waitSize2 := binary.LittleEndian.Uint32(b[36:])
				waitData2 := b[52:]

				if uint32(len(waitData2)) != waitSize2 {
					continue // shouldn't happened for audio
				}

				packetData := make([]byte, waitSize2)
				copy(packetData, waitData2)

				select {
				case c.rawPkt <- packetData:
				default:
					c.err = fmt.Errorf("%s: media queue is full", "tutk")
					return
				}
				continue
			}
		case 1:
			switch string(b[28:30]) {
			case "\x00\x00":
				_ = c.Write(msgAckCh1Req0000(b))
				continue
			case "\x00\x07":
				_ = c.Write(msgAckCh1Req0007(b))
				continue
			}
		case 5:
			if len(b) == 48 {
				_ = c.Write(msgAckCh5(b))
				continue
			}
		}

		fmt.Printf("%s: unknown msg: %x\n", "tutk", buf[:n])
	}
}

func (c *Conn) Write(req []byte) error {
	//log.Printf("-> %x", req)
	_, err := c.conn.WriteToUDP(TransCodePartial(req), c.addr)
	return err
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
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}

		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue // skip messages from another IP
		}

		res := ReverseTransCodePartial(buf[:n])
		//log.Printf("<- %x", b)
		if ok(req, res) {
			c.addr.Port = addr.Port
			return res, nil
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

	req := c.msgAvSendIOCtrl(cmd, data)

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

func (c *Conn) msgLanSearch(uid []byte, i byte) []byte {
	const size = 68 // or 52 or 68 or 88
	b := make([]byte, size)
	copy(b, "\x04\x02\x0f\x02")
	b[4] = size - 16
	copy(b[8:], "\x01\x06\x21\x00")
	copy(b[16:], uid)
	copy(b[52:], "\x00\x03\x01\x02") // or 07000303 or 01010204
	copy(b[56:], c.sid[8:])
	b[64] = i
	return b
}

func (c *Conn) msg(size uint16) []byte {
	b := make([]byte, size)
	copy(b, "\x04\x02\x19\x0a")
	binary.LittleEndian.PutUint16(b[4:], size-16)
	binary.LittleEndian.PutUint16(b[6:], c.seqCh0)
	c.seqCh0++ // start from 0
	copy(b[8:], "\x07\x04\x21\x00")
	return b
}

func (c *Conn) msgAvClientStartReq() []byte {
	const size = 586 // or 586 or 598
	b := c.msg(size)
	copy(b[12:], c.sid)
	copy(b[28:], "\x00\x00\x08\x00") // or 00000400 or 00000b00
	binary.LittleEndian.PutUint16(b[44:], size-52)
	binary.LittleEndian.PutUint32(b[48:], uint32(time.Now().UnixMilli()))
	copy(b[size-16:], "\x04\x00\x00\x00\xfb\x07\x1f\x00")
	return b
}

func (c *Conn) msgAvSendIOCtrl(cmd uint16, msg []byte) []byte {
	size := 52 + 4 + uint16(len(msg))
	b := c.msg(size)
	copy(b[12:], c.sid)
	copy(b[28:], "\x00\x70\x08\x00") // or 00700400 or 00700b00
	c.seqCmd++                       // start from 1
	binary.LittleEndian.PutUint16(b[32:], c.seqCmd)
	binary.LittleEndian.PutUint16(b[44:], size-52)
	//_, _ = rand.Read(b[48:52]) // mid
	binary.LittleEndian.PutUint32(b[48:], uint32(time.Now().UnixMilli()))
	binary.LittleEndian.PutUint16(b[52:], cmd)
	copy(b[56:], msg)
	return b
}

const version = 0x19

func msgAckPing(req []byte) []byte {
	// <- [24] 0402120a 08000000 28041200 000000005b0d4202070aa8c0
	// -> [24] 04021a0a 08000000 27042100 000000005b0d4202070aa8c0
	req[2] = version
	req[8] = 0x27
	req[10] = 0x21
	return req
}

func msgAck(req []byte, size byte) []byte {
	// xxxx??xx ??00xxxx 07xx21xx ...
	req[2] = version
	req[4] = size - 16
	req[5] = 0x00
	req[8] = 0x07
	req[10] = 0x21
	return req[:size]
}

func (c *Conn) msgAckCh0Req0012(req []byte) []byte {
	// <- [64] 0402120a 30000000 08041200 e6e8 0000 0c000000e6e839da66b0dc14 00120800000000000000000000000000 0c00 000000000000 020000000100000001000000
	// -> [72] 0402190a 38000300 07042100 e6e8 0000 0c000000e6e839da66b0dc14 00130b00000000000000000000000000 1400 000000000000 0200000001000000010000000000000000000000
	const size = 72
	req = append(req, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint16(req[6:], c.seqCh0) // channel sequence
	c.seqCh0++
	req[28] = 0x00 // command
	req[29] = 0x13
	req[44] = size - 52 // data size
	req[45] = 0x00
	return msgAck(req, size)
}

func (c *Conn) msgAckCh0Req0070(req []byte) []byte {
	// <- [104] 0402120a 58000300 08041200 e6e8 0000 0c000000e6e839da66b0dc14 00700800010000000000000000000000 3400 00007625a02f ...
	// -> [ 52] 0402190a 24000400 07042100 e6e8 0000 0c000000e6e839da66b0dc14 00710800010000000000000000000000 0000 00007625a02f
	binary.LittleEndian.PutUint16(req[6:], c.seqCh0) // channel sequence
	c.seqCh0++
	req[28] = 0x00 // command
	req[29] = 0x71
	req[44] = 0x00 // data size
	req[45] = 0x00
	return msgAck(req, 52)
}

func msgAckCh1Req0000(req []byte) []byte {
	// <- [590] 0402120a 3e020100 08041200 e6e8 0100 0c000000e6e839da66b0dc14 00000800000000000000000000000000 1a02 0000d9c0001b ...
	// -> [ 84] 0402190a 44000000 07042100 e6e8 0100 0c000000e6e839da66b0dc14 00140b00000000000000000000000000 2000 0000d9c0001b ...
	const size = 84
	req[28] = 0x00 // command
	req[29] = 0x14
	req[44] = size - 52 // data size
	req[45] = 0x00
	copy(req[52:], req[len(req)-32:]) // size
	return msgAck(req, size)
}

func msgAckCh1Req0007(req []byte) []byte {
	// <- [64] 0402120a 30000300 08041200 e6e8 0100 0c000000e6e839da66b0dc14 00070800000000000000000000000000 0c00 000001000000 000000006f1ea02f00000000
	// -> [56] 0402190a 28000200 07042100 e6e8 0100 0c000000e6e839da66b0dc14 010a0b00000000000000000000000000 0000 000001000000 00000000
	req[28] = 0x01 // command
	req[29] = 0x0a
	req[44] = 0x00 // data size
	req[45] = 0x00
	return msgAck(req, 56)
}

func msgAckCh5(req []byte) []byte {
	// <- [48] 0402120a 20000200 08041200 e6e8 0500 0c000000e6e839da66b0dc14 5a97c2f1010500000000000000000000 00a0 0000
	// -> [48] 0402190a 20000200 07042100 e6e8 0500 0c000000e6e839da66b0dc14 5a97c2f1410500000000000000000000 00a0 0000
	req[32] = 0x41
	return msgAck(req, 48)
}
