package dtls

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/pion/dtls/v3"
)

const (
	magicCC51    = "\x51\xcc"         // (wyze specific?)
	sdkVersion42 = "\x01\x01\x02\x04" // 4.2.1.1
	sdkVersion43 = "\x00\x08\x03\x04" // 4.3.8.0
)

const (
	cmdDiscoReq     uint16 = 0x0601
	cmdDiscoRes     uint16 = 0x0602
	cmdSessionReq   uint16 = 0x0402
	cmdSessionRes   uint16 = 0x0404
	cmdDataTX       uint16 = 0x0407
	cmdDataRX       uint16 = 0x0408
	cmdKeepaliveReq uint16 = 0x0427
	cmdKeepaliveRes uint16 = 0x0428

	headerSize    = 16
	discoBodySize = 72
	discoSize     = headerSize + discoBodySize
	sessionBody   = 36
	sessionSize   = headerSize + sessionBody
)

const (
	cmdDiscoCC51      uint16 = 0x1002
	cmdKeepaliveCC51  uint16 = 0x1202
	cmdDTLSCC51       uint16 = 0x1502
	payloadSizeCC51   uint16 = 0x0028
	packetSizeCC51           = 52
	headerSizeCC51           = 28
	authSizeCC51             = 20
	keepaliveSizeCC51        = 48
)

const (
	magicAVLoginResp uint16 = 0x2100
	magicIOCtrl      uint16 = 0x7000
	magicChannelMsg  uint16 = 0x1000
	magicACK         uint16 = 0x0009
	magicAVLogin1    uint16 = 0x0000
	magicAVLogin2    uint16 = 0x2000
)

const (
	protoVersion uint16 = 0x000c
	defaultCaps  uint32 = 0x001f07fb
)

const (
	iotcChannelMain = 0 // Main AV (we = DTLS Client)
	iotcChannelBack = 1 // Backchannel (we = DTLS Server)
)

type DTLSConn struct {
	conn    *net.UDPConn
	addr    *net.UDPAddr
	frames  *tutk.FrameHandler
	err     error
	verbose bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex

	// DTLS
	clientConn *dtls.Conn
	serverConn *dtls.Conn
	clientBuf  chan []byte
	serverBuf  chan []byte
	rawCmd     chan []byte

	// Identity
	uid     string
	authKey string
	enr     string
	psk     []byte

	// Session
	sid                []byte
	ticket             uint16
	hasTwoWayStreaming bool

	// Protocol
	isCC51       bool
	seq          uint16
	seqCmd       uint16
	avSeq        uint32
	kaSeq        uint32
	audioSeq     uint32
	audioFrameNo uint32

	// Ack
	ackFlags   uint16
	rxSeqStart uint16
	rxSeqEnd   uint16
	rxSeqInit  bool
	cmdAck     func()
}

func DialDTLS(host string, port int, uid, authKey, enr string, verbose bool) (*DTLSConn, error) {
	udp, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	_ = udp.SetReadBuffer(2 * 1024 * 1024)

	ctx, cancel := context.WithCancel(context.Background())
	psk := DerivePSK(enr)

	if port == 0 {
		port = 32761
	}

	c := &DTLSConn{
		conn:       udp,
		addr:       &net.UDPAddr{IP: net.ParseIP(host), Port: port},
		uid:        uid,
		authKey:    authKey,
		enr:        enr,
		psk:        psk,
		verbose:    verbose,
		ctx:        ctx,
		cancel:     cancel,
		rxSeqStart: 0xffff,
		rxSeqEnd:   0xffff,
	}

	if err = c.discovery(); err != nil {
		_ = c.Close()
		return nil, err
	}

	c.clientBuf = make(chan []byte, 64)
	c.serverBuf = make(chan []byte, 64)
	c.rawCmd = make(chan []byte, 16)
	c.frames = tutk.NewFrameHandler(c.verbose)

	c.wg.Add(1)
	go c.reader()

	if err = c.connect(); err != nil {
		_ = c.Close()
		return nil, err
	}

	c.wg.Add(1)
	go c.worker()

	return c, nil
}

func (c *DTLSConn) AVClientStart(timeout time.Duration) error {
	randomID := tutk.GenSessionID()
	pkt1 := c.msgAVLogin(magicAVLogin1, 570, 0x0001, randomID)
	pkt2 := c.msgAVLogin(magicAVLogin2, 572, 0x0000, randomID)
	pkt2[20]++ // pkt2 has randomID incremented by 1

	if _, err := c.clientConn.Write(pkt1); err != nil {
		return fmt.Errorf("av login 1 failed: %w", err)
	}

	time.Sleep(10 * time.Millisecond)

	if _, err := c.clientConn.Write(pkt2); err != nil {
		return fmt.Errorf("av login 2 failed: %w", err)
	}

	// Wait for response
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case data, ok := <-c.rawCmd:
			if !ok {
				return io.EOF
			}
			if len(data) >= 32 && binary.LittleEndian.Uint16(data) == magicAVLoginResp {
				c.hasTwoWayStreaming = data[31] == 1

				ack := c.msgACK()
				c.clientConn.Write(ack)

				// Start ACK sender for continuous streaming
				c.wg.Add(1)
				go func() {
					defer c.wg.Done()
					ackTicker := time.NewTicker(100 * time.Millisecond)
					defer ackTicker.Stop()

					for {
						select {
						case <-c.ctx.Done():
							return
						case <-ackTicker.C:
							if c.clientConn != nil {
								ack := c.msgACK()
								c.clientConn.Write(ack)
							}
						}
					}
				}()

				return nil
			}
		case <-timer.C:
			return context.DeadlineExceeded
		}
	}
}

func (c *DTLSConn) AVServStart() error {
	conn, err := NewDTLSServer(c.ctx, iotcChannelBack, c.addr, c.WriteDTLS, c.serverBuf, c.psk)
	if err != nil {
		return fmt.Errorf("dtls: server handshake failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("[DTLS] Server handshake complete on channel %d\n", iotcChannelBack)
		fmt.Printf("[SERVER] Waiting for AV Login request from camera...\n")
	}

	// Wait for AV Login request from camera
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		go conn.Close()
		return fmt.Errorf("read av login: %w", err)
	}

	if c.verbose {
		fmt.Printf("[SERVER] AV Login request len=%d data:\n%s", n, hexDump(buf[:n]))
	}

	if n < 24 {
		go conn.Close()
		return fmt.Errorf("av login too short: %d bytes", n)
	}

	checksum := binary.LittleEndian.Uint32(buf[20:])
	resp := c.msgAVLoginResponse(checksum)

	if c.verbose {
		fmt.Printf("[SERVER] Sending AV Login response: %d bytes\n", len(resp))
	}

	if _, err = conn.Write(resp); err != nil {
		go conn.Close()
		return fmt.Errorf("write av login response: %w", err)
	}

	if c.verbose {
		fmt.Printf("[SERVER] AV Login response sent, waiting for possible resend...\n")
	}

	// Camera may resend, respond again
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if n, _ = conn.Read(buf); n > 0 {
		if c.verbose {
			fmt.Printf("[SERVER] Received AV Login resend: %d bytes\n", n)
		}
		conn.Write(resp)
	}

	conn.SetReadDeadline(time.Time{})

	if c.verbose {
		fmt.Printf("[SERVER] AV Login complete, ready for two way streaming\n")
	}

	c.mu.Lock()
	c.serverConn = conn
	c.mu.Unlock()

	return nil
}

func (c *DTLSConn) AVServStop() error {
	c.mu.Lock()
	serverConn := c.serverConn
	c.serverConn = nil

	// Reset audio TX state
	c.audioSeq = 0
	c.audioFrameNo = 0
	c.mu.Unlock()

	if serverConn == nil {
		return nil
	}

	go serverConn.Close()

	return nil
}

func (c *DTLSConn) AVRecvFrameData() (*tutk.Packet, error) {
	select {
	case pkt, ok := <-c.frames.Recv():
		if !ok {
			return nil, c.Error()
		}
		return pkt, nil
	case <-c.ctx.Done():
		return nil, c.Error()
	}
}

func (c *DTLSConn) AVSendAudioData(codec byte, payload []byte, timestampUS uint32, sampleRate uint32, channels uint8) error {
	c.mu.Lock()
	conn := c.serverConn
	if conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("av server not ready")
	}

	frame := c.msgAudioFrame(payload, timestampUS, codec, sampleRate, channels)

	c.mu.Unlock()

	n, err := conn.Write(frame)
	if c.verbose {
		if err != nil {
			fmt.Printf("[SERVER TX] DTLS Write ERROR: %v\n", err)
		} else {
			fmt.Printf("[SERVER TX] len=%d, data:\n%s", n, hexDump(frame))
		}
	}
	return err
}

func (c *DTLSConn) Write(data []byte) error {
	if c.isCC51 {
		_, err := c.conn.WriteToUDP(data, c.addr)
		return err
	}
	_, err := c.conn.WriteToUDP(tutk.TransCodeBlob(data), c.addr)
	return err
}

func (c *DTLSConn) WriteDTLS(payload []byte, channel byte) error {
	var frame []byte
	if c.isCC51 {
		frame = c.msgTxDataCC51(payload, channel)
	} else {
		frame = c.msgTxData(payload, channel)
	}

	return c.Write(frame)
}

func (c *DTLSConn) WriteIOCtrl(payload []byte) error {
	_, err := c.conn.Write(c.msgIOCtrl(payload))
	return err
}

func (c *DTLSConn) WriteAndWait(req []byte, ok func(res []byte) bool) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if err := c.Write(req); err == nil && t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	_ = c.conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer c.conn.SetDeadline(time.Time{})

	buf := make([]byte, 2048)
	for {
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}
		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue
		}

		var res []byte
		if c.isCC51 {
			res = buf[:n]
		} else {
			res = tutk.ReverseTransCodeBlob(buf[:n])
		}

		if ok(res) {
			c.addr.Port = addr.Port
			return res, nil
		}
	}
}

func (c *DTLSConn) WriteAndWaitIOCtrl(payload []byte, match func([]byte) bool, timeout time.Duration) ([]byte, error) {
	frame := c.msgIOCtrl(payload)
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		c.mu.RLock()
		conn := c.clientConn
		c.mu.RUnlock()
		if conn != nil {
			if _, err := conn.Write(frame); err == nil && t != nil {
				t.Reset(time.Second)
			}
		}
	})
	defer t.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case data, ok := <-c.rawCmd:
			if !ok {
				return nil, io.EOF
			}

			ack := c.msgACK()
			c.clientConn.Write(ack)

			if match(data) {
				return data, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for response")
		}
	}
}

func (c *DTLSConn) HasTwoWayStreaming() bool {
	return c.hasTwoWayStreaming
}

func (c *DTLSConn) IsBackchannelReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.serverConn != nil
}

func (c *DTLSConn) RemoteAddr() *net.UDPAddr {
	return c.addr
}

func (c *DTLSConn) LocalAddr() *net.UDPAddr {
	return c.conn.LocalAddr().(*net.UDPAddr)
}

func (c *DTLSConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *DTLSConn) Close() error {
	c.cancel()

	c.mu.Lock()
	if conn := c.serverConn; conn != nil {
		c.serverConn = nil
		go conn.Close()
	}
	if conn := c.clientConn; conn != nil {
		c.clientConn = nil
		go conn.Close()
	}
	if c.frames != nil {
		c.frames.Close()
	}
	c.mu.Unlock()

	c.wg.Wait()

	return c.conn.Close()
}

func (c *DTLSConn) Error() error {
	if c.err != nil {
		return c.err
	}
	return io.EOF
}

func (c *DTLSConn) discovery() error {
	c.sid = tutk.GenSessionID()

	pktIOTC := tutk.TransCodeBlob(c.msgDisco(1))
	pktCC51 := c.msgDiscoCC51(0, 0, false)

	buf := make([]byte, 2048)
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		c.conn.WriteToUDP(pktIOTC, c.addr)
		c.conn.WriteToUDP(pktCC51, c.addr)

		c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if !addr.IP.Equal(c.addr.IP) {
			continue
		}

		// CC51 protocol
		if n >= packetSizeCC51 && string(buf[:2]) == magicCC51 {
			if binary.LittleEndian.Uint16(buf[4:]) == cmdDiscoCC51 {
				c.addr, c.isCC51, c.ticket = addr, true, binary.LittleEndian.Uint16(buf[14:])
				if n >= 24 {
					copy(c.sid, buf[16:24])
				}
				return c.discoDoneCC51()
			}
			continue
		}

		// IOTC Protocol (Basis)
		data := tutk.ReverseTransCodeBlob(buf[:n])
		if len(data) >= 16 && binary.LittleEndian.Uint16(data[8:]) == cmdDiscoRes {
			c.addr, c.isCC51 = addr, false
			return c.discoDone()
		}
	}

	return fmt.Errorf("discovery timeout")
}

func (c *DTLSConn) discoDone() error {
	c.Write(c.msgDisco(2))
	time.Sleep(100 * time.Millisecond)
	_, err := c.WriteAndWait(c.msgSession(), func(res []byte) bool {
		return len(res) >= 16 && binary.LittleEndian.Uint16(res[8:]) == cmdSessionRes
	})
	return err
}

func (c *DTLSConn) discoDoneCC51() error {
	_, err := c.WriteAndWait(c.msgDiscoCC51(2, c.ticket, false), func(res []byte) bool {
		if len(res) < packetSizeCC51 || string(res[:2]) != magicCC51 {
			return false
		}
		cmd := binary.LittleEndian.Uint16(res[4:])
		dir := binary.LittleEndian.Uint16(res[8:])
		seq := binary.LittleEndian.Uint16(res[12:])
		return cmd == cmdDiscoCC51 && dir == 0xFFFF && seq == 3
	})
	return err
}

func (c *DTLSConn) connect() error {
	conn, err := NewDTLSClient(c.ctx, iotcChannelMain, c.addr, c.WriteDTLS, c.clientBuf, c.psk)
	if err != nil {
		return fmt.Errorf("dtls: client handshake failed: %w", err)
	}

	c.mu.Lock()
	c.clientConn = conn
	c.mu.Unlock()

	if c.verbose {
		fmt.Printf("[DTLS] Client handshake complete on channel %d\n", iotcChannelMain)
	}

	return nil
}

func (c *DTLSConn) worker() {
	defer c.wg.Done()

	buf := make([]byte, 2048)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		n, err := c.clientConn.Read(buf)
		if err != nil {
			c.err = err
			return
		}

		if n < 2 {
			continue
		}

		data := buf[:n]
		magic := binary.LittleEndian.Uint16(data)

		if c.verbose {
			fmt.Printf("[DTLS RX] magic=0x%04x len=%d\n", magic, n)
		}

		switch magic {
		case magicAVLoginResp:
			c.queue(c.rawCmd, data)

		case magicIOCtrl, magicChannelMsg:
			c.queue(c.rawCmd, data)

		case protoVersion:
			// Seq-Tracking
			if len(data) >= 8 {
				seq := binary.LittleEndian.Uint16(data[4:])
				if !c.rxSeqInit {
					c.rxSeqInit = true
				}
				if seq > c.rxSeqEnd || c.rxSeqEnd == 0xffff {
					c.rxSeqEnd = seq
				}
			}
			c.queue(c.rawCmd, data)

		case magicACK:
			c.mu.RLock()
			ack := c.cmdAck
			c.mu.RUnlock()
			if ack != nil {
				ack()
			}

		default:
			channel := data[0]
			if channel == tutk.ChannelAudio || channel == tutk.ChannelIVideo || channel == tutk.ChannelPVideo {
				c.frames.Handle(data)
			}
		}
	}
}

func (c *DTLSConn) reader() {
	defer c.wg.Done()

	buf := make([]byte, 2048)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if !addr.IP.Equal(c.addr.IP) {
			if c.verbose {
				fmt.Printf("Ignored packet from unknown IP: %s\n", addr.IP.String())
			}
			continue
		}
		if addr.Port != c.addr.Port {
			c.addr.Port = addr.Port
		}

		// CC51 Protocol
		if c.isCC51 && n >= 12 && string(buf[:2]) == magicCC51 {
			cmd := binary.LittleEndian.Uint16(buf[4:])
			switch cmd {
			case cmdKeepaliveCC51:
				if n >= keepaliveSizeCC51 {
					_ = c.Write(c.msgKeepaliveCC51())
				}
			case cmdDTLSCC51:
				if n >= headerSizeCC51+authSizeCC51 {
					ch := byte(binary.LittleEndian.Uint16(buf[12:]) >> 8)
					dtlsData := buf[headerSizeCC51 : n-authSizeCC51]
					switch ch {
					case iotcChannelMain:
						c.queue(c.clientBuf, dtlsData)
					case iotcChannelBack:
						c.queue(c.serverBuf, dtlsData)
					}
				}
			}
			continue
		}

		// IOTC Protocol (Basis)
		data := tutk.ReverseTransCodeBlob(buf[:n])
		if len(data) < 16 {
			continue
		}

		switch binary.LittleEndian.Uint16(data[8:]) {
		case cmdKeepaliveRes:
			if len(data) > 24 {
				_ = c.Write(c.msgKeepalive(data[16:]))
			}
		case cmdDataRX:
			if len(data) > 28 {
				ch := data[14]
				switch ch {
				case iotcChannelMain:
					c.queue(c.clientBuf, data[28:])
				case iotcChannelBack:
					c.queue(c.serverBuf, data[28:])
				}
			}
		}
	}
}

func (c *DTLSConn) queue(ch chan []byte, data []byte) {
	b := make([]byte, len(data))
	copy(b, data)
	select {
	case ch <- b:
	default:
		select {
		case <-ch:
		default:
		}
		ch <- b
	}
}

func (c *DTLSConn) msgDisco(stage byte) []byte {
	b := make([]byte, discoSize)
	copy(b, "\x04\x02\x1a\x02")                         // marker + mode
	binary.LittleEndian.PutUint16(b[4:], discoBodySize) // body size
	binary.LittleEndian.PutUint16(b[8:], cmdDiscoReq)   // 0x0601
	binary.LittleEndian.PutUint16(b[10:], 0x0021)       // flags
	body := b[headerSize:]
	copy(body[:20], c.uid)
	copy(body[36:], sdkVersion42) // SDK 4.2.1.1
	copy(body[40:], c.sid)
	body[48] = stage
	if stage == 1 && len(c.authKey) > 0 {
		copy(body[58:], c.authKey)
	}
	return b
}

func (c *DTLSConn) msgDiscoCC51(seq, ticket uint16, isResponse bool) []byte {
	b := make([]byte, packetSizeCC51)
	copy(b[:2], magicCC51)
	binary.LittleEndian.PutUint16(b[4:], cmdDiscoCC51)    // 0x1002
	binary.LittleEndian.PutUint16(b[6:], payloadSizeCC51) // 40 bytes
	if isResponse {
		binary.LittleEndian.PutUint16(b[8:], 0xFFFF) // response
	}
	binary.LittleEndian.PutUint16(b[12:], seq)
	binary.LittleEndian.PutUint16(b[14:], ticket)
	copy(b[16:24], c.sid)
	copy(b[24:28], sdkVersion43) // SDK 4.3.8.0
	b[28] = 0x1d                 // unknown field (capability/build flag?)
	h := hmac.New(sha1.New, append([]byte(c.uid), c.authKey...))
	h.Write(b[:32])
	copy(b[32:52], h.Sum(nil))
	return b
}

func (c *DTLSConn) msgKeepaliveCC51() []byte {
	c.kaSeq += 2
	b := make([]byte, keepaliveSizeCC51)
	copy(b[:2], magicCC51)
	binary.LittleEndian.PutUint16(b[4:], cmdKeepaliveCC51) // 0x1202
	binary.LittleEndian.PutUint16(b[6:], 0x0024)           // 36 bytes payload
	binary.LittleEndian.PutUint32(b[16:], c.kaSeq)         // counter
	copy(b[20:28], c.sid)                                  // session ID
	h := hmac.New(sha1.New, append([]byte(c.uid), c.authKey...))
	h.Write(b[:28])
	copy(b[28:48], h.Sum(nil))
	return b
}

func (c *DTLSConn) msgSession() []byte {
	b := make([]byte, sessionSize)
	copy(b, "\x04\x02\x1a\x02")                         // marker + mode
	binary.LittleEndian.PutUint16(b[4:], sessionBody)   // body size
	binary.LittleEndian.PutUint16(b[8:], cmdSessionReq) // 0x0402
	binary.LittleEndian.PutUint16(b[10:], 0x0033)       // flags
	body := b[headerSize:]
	copy(body[:20], c.uid)
	copy(body[20:], c.sid)
	binary.LittleEndian.PutUint32(body[32:], uint32(time.Now().Unix()))
	return b
}

func (c *DTLSConn) msgAVLogin(magic uint16, size int, flags uint16, randomID []byte) []byte {
	b := make([]byte, size)
	binary.LittleEndian.PutUint16(b, magic)
	binary.LittleEndian.PutUint16(b[2:], protoVersion)
	binary.LittleEndian.PutUint16(b[16:], uint16(size-24)) // payload size
	binary.LittleEndian.PutUint16(b[18:], flags)
	copy(b[20:], randomID[:4])
	copy(b[24:], "admin")                               // username
	copy(b[280:], c.enr)                                // password/ENR
	binary.LittleEndian.PutUint32(b[540:], 4)           // security_mode ?
	binary.LittleEndian.PutUint32(b[552:], defaultCaps) // capabilities
	return b
}

func (c *DTLSConn) msgAVLoginResponse(checksum uint32) []byte {
	b := make([]byte, 60)
	binary.LittleEndian.PutUint16(b, 0x2100)        // magic
	binary.LittleEndian.PutUint16(b[2:], 0x000c)    // version
	b[4] = 0x10                                     // success
	binary.LittleEndian.PutUint32(b[16:], 0x24)     // payload size
	binary.LittleEndian.PutUint32(b[20:], checksum) // echo checksum
	b[29] = 0x01                                    // enable flag
	b[31] = 0x01                                    // two-way streaming
	binary.LittleEndian.PutUint32(b[36:], 0x04)     // buffer config
	binary.LittleEndian.PutUint32(b[40:], defaultCaps)
	binary.LittleEndian.PutUint16(b[54:], 0x0003) // channel info
	binary.LittleEndian.PutUint16(b[56:], 0x0002)
	return b
}

func (c *DTLSConn) msgAudioFrame(payload []byte, timestampUS uint32, codec byte, sampleRate uint32, channels uint8) []byte {
	c.audioSeq++
	c.audioFrameNo++
	prevFrame := uint32(0)
	if c.audioFrameNo > 1 {
		prevFrame = c.audioFrameNo - 1
	}

	totalPayload := len(payload) + 16 // payload + frameinfo
	b := make([]byte, 36+totalPayload)

	// Outer header (36 bytes)
	b[0] = tutk.ChannelAudio      // 0x03
	b[1] = tutk.FrameTypeStartAlt // 0x09
	binary.LittleEndian.PutUint16(b[2:], protoVersion)
	binary.LittleEndian.PutUint32(b[4:], c.audioSeq)
	binary.LittleEndian.PutUint32(b[8:], timestampUS)
	if c.audioFrameNo == 1 {
		binary.LittleEndian.PutUint32(b[12:], 0x00000001)
	} else {
		binary.LittleEndian.PutUint32(b[12:], 0x00100001)
	}

	// Inner header
	b[16] = tutk.ChannelAudio
	b[17] = tutk.FrameTypeEndSingle
	binary.LittleEndian.PutUint16(b[18:], uint16(prevFrame))
	binary.LittleEndian.PutUint16(b[20:], 0x0001) // pkt_total
	binary.LittleEndian.PutUint16(b[22:], 0x0010) // flags
	binary.LittleEndian.PutUint32(b[24:], uint32(totalPayload))
	binary.LittleEndian.PutUint32(b[28:], prevFrame)
	binary.LittleEndian.PutUint32(b[32:], c.audioFrameNo)
	copy(b[36:], payload) // Payload + FrameInfo
	fi := b[36+len(payload):]
	fi[0] = codec // Codec ID (low byte)
	fi[1] = 0     // Codec ID (high byte, unused)
	// Audio flags: [3:2]=sampleRateIdx [1]=16bit [0]=stereo
	srIdx := tutk.GetSampleRateIndex(sampleRate)
	fi[2] = (srIdx << 2) | 0x02 // 16-bit always set
	if channels == 2 {
		fi[2] |= 0x01
	}
	fi[4] = 1 // online
	binary.LittleEndian.PutUint32(fi[12:], (c.audioFrameNo-1)*tutk.GetSamplesPerFrame(codec)*1000/sampleRate)
	return b
}

func (c *DTLSConn) msgTxData(payload []byte, channel byte) []byte {
	bodySize := 12 + len(payload)
	b := make([]byte, 16+bodySize)
	copy(b, "\x04\x02\x1a\x0b")                            // marker + mode=data
	binary.LittleEndian.PutUint16(b[4:], uint16(bodySize)) // body size
	binary.LittleEndian.PutUint16(b[6:], c.seq)            // sequence
	c.seq++
	binary.LittleEndian.PutUint16(b[8:], cmdDataTX)   // 0x0407
	binary.LittleEndian.PutUint16(b[10:], 0x0021)     // flags
	copy(b[12:], c.sid[:2])                           // rid[0:2]
	b[14] = channel                                   // channel
	b[15] = 0x01                                      // marker
	binary.LittleEndian.PutUint32(b[16:], 0x0000000c) // const
	copy(b[20:], c.sid[:8])                           // rid
	copy(b[28:], payload)
	return b
}

func (c *DTLSConn) msgTxDataCC51(payload []byte, channel byte) []byte {
	payloadSize := uint16(16 + len(payload) + authSizeCC51)
	b := make([]byte, headerSizeCC51+len(payload)+authSizeCC51)
	copy(b[:2], magicCC51)
	binary.LittleEndian.PutUint16(b[4:], cmdDTLSCC51) // 0x1502
	binary.LittleEndian.PutUint16(b[6:], payloadSize)
	binary.LittleEndian.PutUint16(b[12:], uint16(0x0010)|(uint16(channel)<<8)) // channel in high byte
	binary.LittleEndian.PutUint16(b[14:], c.ticket)
	copy(b[16:24], c.sid)
	binary.LittleEndian.PutUint32(b[24:], 1) // const
	copy(b[headerSizeCC51:], payload)
	h := hmac.New(sha1.New, append([]byte(c.uid), c.authKey...))
	h.Write(b[:headerSizeCC51])
	copy(b[headerSizeCC51+len(payload):], h.Sum(nil))
	return b
}

func (c *DTLSConn) msgACK() []byte {
	c.ackFlags++
	b := make([]byte, 24)
	binary.LittleEndian.PutUint16(b[0:], magicACK)     // 0x0009
	binary.LittleEndian.PutUint16(b[2:], protoVersion) // 0x000c
	binary.LittleEndian.PutUint32(b[4:], c.avSeq)      // TX seq
	c.avSeq++
	binary.LittleEndian.PutUint16(b[8:], c.rxSeqStart) // RX start (last acked)
	binary.LittleEndian.PutUint16(b[10:], c.rxSeqEnd)  // RX end (highest received)
	if c.rxSeqInit {
		c.rxSeqStart = c.rxSeqEnd
	}
	binary.LittleEndian.PutUint16(b[12:], c.ackFlags)             // AckFlags
	binary.LittleEndian.PutUint32(b[16:], uint32(c.ackFlags)<<16) // AckCounter
	ts := uint32(time.Now().UnixMilli() & 0xFFFF)
	binary.LittleEndian.PutUint16(b[20:], uint16(ts)) // Timestamp
	return b
}

func (c *DTLSConn) msgKeepalive(incoming []byte) []byte {
	b := make([]byte, 24)
	copy(b, "\x04\x02\x1a\x0a")                           // marker + mode
	binary.LittleEndian.PutUint16(b[4:], 8)               // body size
	binary.LittleEndian.PutUint16(b[8:], cmdKeepaliveReq) // 0x0427
	binary.LittleEndian.PutUint16(b[10:], 0x0021)         // flags
	if len(incoming) >= 8 {
		copy(b[16:], incoming[:8]) // echo payload
	}
	return b
}

func (c *DTLSConn) msgIOCtrl(payload []byte) []byte {
	b := make([]byte, 40+len(payload))
	binary.LittleEndian.PutUint16(b, protoVersion)     // magic
	binary.LittleEndian.PutUint16(b[2:], protoVersion) // version
	binary.LittleEndian.PutUint32(b[4:], c.avSeq)      // av seq
	c.avSeq++
	binary.LittleEndian.PutUint16(b[16:], magicIOCtrl)            // 0x7000
	binary.LittleEndian.PutUint16(b[18:], c.seqCmd)               // sub channel
	binary.LittleEndian.PutUint32(b[20:], 1)                      // ioctl seq
	binary.LittleEndian.PutUint32(b[24:], uint32(len(payload)+4)) // payload size
	binary.LittleEndian.PutUint32(b[28:], uint32(c.seqCmd))       // flag
	b[37] = 0x01
	copy(b[40:], payload)
	c.seqCmd++
	return b
}

func hexDump(data []byte) string {
	const maxBytes = 650
	totalLen := len(data)
	truncated := totalLen > maxBytes
	if truncated {
		data = data[:maxBytes]
	}

	var result string
	for i := 0; i < len(data); i += 16 {
		end := min(i+16, len(data))
		line := fmt.Sprintf("    %04x:", i)
		for j := i; j < end; j++ {
			line += fmt.Sprintf(" %02x", data[j])
		}
		result += line + "\n"
	}

	if truncated {
		result += fmt.Sprintf("    ... (truncated, showing %d of %d bytes)\n", maxBytes, totalLen)
	}
	return result
}
