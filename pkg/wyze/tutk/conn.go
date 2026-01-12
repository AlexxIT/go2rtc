package tutk

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/wyze/crypto"
	"github.com/pion/dtls/v3"
)

const (
	MaxPacketSize    = 2048
	ReadBufferSize   = 2 * 1024 * 1024
	DiscoTimeout     = 5000 * time.Millisecond
	DiscoInterval    = 100 * time.Millisecond
	SessionTimeout   = 5000 * time.Millisecond
	ReadWaitInterval = 50 * time.Millisecond
)

type Conn struct {
	conn *net.UDPConn
	addr *net.UDPAddr

	// Identity
	uid     string
	authKey string
	enr     string
	mac     string
	psk     []byte
	rid     []byte

	// Session
	sid    []byte
	ticket uint16
	avResp *AVLoginResponse

	// Protocol
	newProto bool
	seq      uint16
	seqCmd   uint16
	avSeq    uint32
	kaSeq    uint32

	// DTLS
	main     *dtls.Conn
	speaker  *dtls.Conn
	mainBuf  chan []byte
	speakBuf chan []byte

	// Channels
	rawCmd chan []byte

	// Audio TX
	audioSeq   uint32
	audioFrame uint32

	// Frame assembly
	frames   *FrameHandler
	ackFlags uint16

	// State
	err     error
	verbose bool

	// Sync
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
	cmdAck func()
}

func Dial(host, uid, authKey, enr, mac string, verbose bool) (*Conn, error) {
	udp, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	_ = udp.SetReadBuffer(ReadBufferSize)

	ctx, cancel := context.WithCancel(context.Background())
	psk := derivePSK(enr)

	if verbose {
		hash := sha256.Sum256([]byte(enr))
		fmt.Printf("[PSK] ENR: %q → SHA256: %x\n", enr, hash)
		fmt.Printf("[PSK] PSK: %x\n", psk)
	}

	c := &Conn{
		conn:    udp,
		addr:    &net.UDPAddr{IP: net.ParseIP(host), Port: DefaultPort},
		rid:     genRandomID(),
		uid:     uid,
		authKey: authKey,
		enr:     enr,
		mac:     mac,
		psk:     psk,
		verbose: verbose,
		ctx:     ctx,
		cancel:  cancel,
	}

	if err = c.discovery(); err != nil {
		_ = c.Close()
		return nil, err
	}

	c.mainBuf = make(chan []byte, 64)
	c.speakBuf = make(chan []byte, 64)
	c.rawCmd = make(chan []byte, 16)
	c.frames = NewFrameHandler(c.verbose)

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

func (c *Conn) AVClientStart(timeout time.Duration) error {
	randomID := genRandomID()
	pkt1 := c.buildAVLoginPacket(MagicAVLogin1, 570, 0x0001, randomID)
	pkt2 := c.buildAVLoginPacket(MagicAVLogin2, 572, 0x0000, randomID)
	pkt2[20]++ // pkt2 has randomID incremented by 1

	if _, err := c.main.Write(pkt1); err != nil {
		return fmt.Errorf("AV login 1 failed: %w", err)
	}

	time.Sleep(50 * time.Millisecond)

	if _, err := c.main.Write(pkt2); err != nil {
		return fmt.Errorf("AV login 2 failed: %w", err)
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
			if len(data) >= 32 && binary.LittleEndian.Uint16(data) == MagicAVLoginResp {
				c.avResp = &AVLoginResponse{
					ServerType:      binary.LittleEndian.Uint32(data[4:]),
					Resend:          int32(data[29]),
					TwoWayStreaming: int32(data[31]),
				}

				if c.verbose {
					fmt.Printf("[TUTK] AV Login Response: two_way_streaming=%d\n", c.avResp.TwoWayStreaming)
				}

				ack := c.buildACK()
				c.main.Write(ack)

				return nil
			}
		case <-timer.C:
			return context.DeadlineExceeded
		}
	}
}

func (c *Conn) AVServStart() error {
	if c.verbose {
		fmt.Printf("[DTLS] Waiting for client handshake on channel %d\n", IOTCChannelBack)
		fmt.Printf("[DTLS] PSK Identity: %s\n", PSKIdentity)
		fmt.Printf("[DTLS] PSK Key: %s\n", hex.EncodeToString(c.psk))
	}

	conn, err := NewDtlsServer(c, IOTCChannelBack, c.psk)
	if err != nil {
		return fmt.Errorf("dtls: server handshake failed: %w", err)
	}

	c.mu.Lock()
	c.speaker = conn
	c.mu.Unlock()

	if c.verbose {
		fmt.Printf("[DTLS] Server handshake complete on channel %d\n", IOTCChannelBack)
	}

	// Wait for and respond to AV Login request from camera
	if err := c.handleSpeakerAVLogin(); err != nil {
		return fmt.Errorf("speaker AV login failed: %w", err)
	}

	return nil
}

func (c *Conn) AVServStop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset audio TX state
	c.audioSeq = 0
	c.audioFrame = 0

	if c.speaker != nil {
		err := c.speaker.Close()
		c.speaker = nil
		return err
	}
	return nil
}

func (c *Conn) AVRecvFrameData() (*Packet, error) {
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

func (c *Conn) AVSendAudioData(codec uint16, payload []byte, timestampUS uint32, sampleRate uint32, channels uint8) error {
	c.mu.Lock()
	conn := c.speaker
	if conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("speaker channel not connected")
	}

	frame := c.buildAudioFrame(payload, timestampUS, codec, sampleRate, channels)

	c.mu.Unlock()

	n, err := conn.Write(frame)
	if c.verbose {
		if err != nil {
			fmt.Printf("[AUDIO TX] DTLS Write ERROR: %v\n", err)
		} else {
			fmt.Printf("[AUDIO TX] DTLS Write OK: %d bytes\n", n)
		}
	}
	return err
}

func (c *Conn) Write(data []byte) error {
	if c.newProto {
		_, err := c.conn.WriteToUDP(data, c.addr)
		return err
	}
	_, err := c.conn.WriteToUDP(crypto.TransCodeBlob(data), c.addr)
	return err
}

func (c *Conn) WriteDTLS(payload []byte, channel byte) error {
	var frame []byte
	if c.newProto {
		frame = c.buildNewTxData(payload, channel)
	} else {
		frame = c.buildTxData(payload, channel)
	}
	return c.Write(frame)
}

func (c *Conn) WriteAndWait(req []byte, timeout time.Duration, ok func(res []byte) bool) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if err := c.Write(req); err == nil && t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	_ = c.conn.SetDeadline(time.Now().Add(timeout))
	defer c.conn.SetDeadline(time.Time{})

	buf := make([]byte, MaxPacketSize)
	for {
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}
		if string(addr.IP) != string(c.addr.IP) || n < 16 {
			continue
		}

		var res []byte
		if c.newProto {
			res = buf[:n]
		} else {
			res = crypto.ReverseTransCodeBlob(buf[:n])
		}

		if ok(res) {
			c.addr.Port = addr.Port
			return res, nil
		}
	}
}

func (c *Conn) WriteAndWaitIOCtrl(cmd uint16, payload []byte, expectCmd uint16, timeout time.Duration) ([]byte, error) {
	frame := c.buildIOCtrlFrame(payload)
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		if _, err := c.main.Write(frame); err == nil && t != nil {
			t.Reset(time.Second)
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

			ack := c.buildACK()
			c.main.Write(ack)

			if len(data) >= 6 {
				if binary.LittleEndian.Uint16(data[4:]) == expectCmd {
					return data, nil
				}
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for K%d", expectCmd)
		}
	}
}

func (c *Conn) GetAVLoginResponse() *AVLoginResponse {
	return c.avResp
}

func (c *Conn) IsBackchannelReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.speaker != nil
}

func (c *Conn) RemoteAddr() *net.UDPAddr {
	return c.addr
}

func (c *Conn) LocalAddr() *net.UDPAddr {
	return c.conn.LocalAddr().(*net.UDPAddr)
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Conn) Close() error {
	c.cancel()

	c.mu.Lock()
	if c.main != nil {
		c.main.Close()
		c.main = nil
	}
	if c.speaker != nil {
		c.speaker.Close()
		c.speaker = nil
	}
	if c.frames != nil {
		c.frames.Close()
	}
	c.mu.Unlock()

	c.wg.Wait()

	return c.conn.Close()
}

func (c *Conn) Error() error {
	if c.err != nil {
		return c.err
	}
	return io.EOF
}

func (c *Conn) discovery() error {
	c.sid = make([]byte, 8)
	rand.Read(c.sid)

	oldPkt := crypto.TransCodeBlob(c.buildDisco(1))
	newPkt := c.buildNewDisco(0, 0, false)
	buf := make([]byte, MaxPacketSize)
	deadline := time.Now().Add(DiscoTimeout)

	for time.Now().Before(deadline) {
		c.conn.WriteToUDP(oldPkt, c.addr)
		c.conn.WriteToUDP(newPkt, c.addr)

		c.conn.SetReadDeadline(time.Now().Add(DiscoInterval))
		n, addr, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if !addr.IP.Equal(c.addr.IP) {
			continue
		}

		// NEW protocol
		if n >= NewPacketSize && binary.LittleEndian.Uint16(buf[:2]) == MagicNewProto {
			if binary.LittleEndian.Uint16(buf[4:]) == CmdNewDisco {
				c.addr, c.newProto, c.ticket = addr, true, binary.LittleEndian.Uint16(buf[14:])
				if n >= 24 {
					copy(c.sid, buf[16:24])
				}
				return c.newDiscoDone()
			}
			continue
		}

		// OLD protocol
		data := crypto.ReverseTransCodeBlob(buf[:n])
		if len(data) >= 16 && binary.LittleEndian.Uint16(data[8:]) == CmdDiscoRes {
			c.addr, c.newProto = addr, false
			return c.oldDiscoDone()
		}
	}

	return fmt.Errorf("discovery timeout")
}

func (c *Conn) oldDiscoDone() error {
	c.Write(c.buildDisco(2))
	time.Sleep(100 * time.Millisecond)

	_, err := c.WriteAndWait(c.buildSession(), SessionTimeout, func(res []byte) bool {
		return len(res) >= 16 && binary.LittleEndian.Uint16(res[8:]) == CmdSessionRes
	})
	return err
}

func (c *Conn) newDiscoDone() error {
	_, err := c.WriteAndWait(c.buildNewDisco(2, c.ticket, false), SessionTimeout, func(res []byte) bool {
		if len(res) < NewPacketSize || binary.LittleEndian.Uint16(res[:2]) != MagicNewProto {
			return false
		}
		cmd := binary.LittleEndian.Uint16(res[4:])
		dir := binary.LittleEndian.Uint16(res[8:])
		seq := binary.LittleEndian.Uint16(res[12:])
		return cmd == CmdNewDisco && dir == 0xFFFF && seq == 3
	})
	return err
}

func (c *Conn) connect() error {
	if c.verbose {
		fmt.Printf("[DTLS] Starting client handshake on channel %d\n", IOTCChannelMain)
		fmt.Printf("[DTLS] PSK Identity: %s\n", PSKIdentity)
		fmt.Printf("[DTLS] PSK Key: %s\n", hex.EncodeToString(c.psk))
	}

	conn, err := NewDtlsClient(c, IOTCChannelMain, c.psk)
	if err != nil {
		return fmt.Errorf("dtls: client create failed: %w", err)
	}

	c.mu.Lock()
	c.main = conn
	c.mu.Unlock()

	if c.verbose {
		fmt.Printf("[DTLS] Client created for channel %d\n", IOTCChannelMain)
	}

	return nil
}

func (c *Conn) worker() {
	defer c.wg.Done()

	buf := make([]byte, 2048)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		n, err := c.main.Read(buf)
		if err != nil {
			c.err = err
			return
		}

		if n < 2 {
			continue
		}

		data := buf[:n]
		magic := binary.LittleEndian.Uint16(data)

		switch magic {
		case MagicAVLoginResp:
			c.queue(c.rawCmd, data)

		case MagicIOCtrl:
			if len(data) >= 32 {
				for i := 32; i+2 < len(data); i++ {
					if data[i] == 'H' && data[i+1] == 'L' {
						c.queue(c.rawCmd, data[i:])
						break
					}
				}
			}

		case MagicChannelMsg:
			if len(data) >= 36 && data[16] == 0x00 {
				for i := 36; i+2 < len(data); i++ {
					if data[i] == 'H' && data[i+1] == 'L' {
						c.queue(c.rawCmd, data[i:])
						break
					}
				}
			}

		case MagicACK:
			c.mu.RLock()
			ack := c.cmdAck
			c.mu.RUnlock()
			if ack != nil {
				ack()
			}

		default:
			channel := data[0]
			if channel == ChannelAudio || channel == ChannelIVideo || channel == ChannelPVideo {
				c.frames.Handle(data)
			}
		}
	}
}

func (c *Conn) reader() {
	defer c.wg.Done()
	buf := make([]byte, MaxPacketSize)

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
			continue
		}
		if addr.Port != c.addr.Port {
			c.addr.Port = addr.Port
		}

		// NEW protocol (0xCC51)
		if c.newProto && n >= 12 && binary.LittleEndian.Uint16(buf[:2]) == MagicNewProto {
			cmd := binary.LittleEndian.Uint16(buf[4:])
			switch cmd {
			case CmdNewKeepalive:
				if n >= NewKeepaliveSize {
					_ = c.Write(c.buildNewKeepalive())
				}
			case CmdNewDTLS:
				if n >= NewHeaderSize+NewAuthSize {
					ch := byte(binary.LittleEndian.Uint16(buf[12:]) >> 8)
					dtls := buf[NewHeaderSize : n-NewAuthSize]
					switch ch {
					case IOTCChannelMain:
						c.queue(c.mainBuf, dtls)
					case IOTCChannelBack:
						c.queue(c.speakBuf, dtls)
					}
				}
			}
			continue
		}

		// OLD protocol (TransCode)
		data := crypto.ReverseTransCodeBlob(buf[:n])
		if len(data) < 16 {
			continue
		}

		switch binary.LittleEndian.Uint16(data[8:]) {
		case CmdKeepaliveRes:
			if len(data) > 24 {
				_ = c.Write(c.buildKeepAlive(data[16:]))
			}
		case CmdDataRX:
			if len(data) > 28 {
				ch := data[14]
				switch ch {
				case IOTCChannelMain:
					c.queue(c.mainBuf, data[28:])
				case IOTCChannelBack:
					c.queue(c.speakBuf, data[28:])
				}
			}
		}
	}
}

func (c *Conn) queue(ch chan []byte, data []byte) {
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

func (c *Conn) handleSpeakerAVLogin() error {
	if c.verbose {
		fmt.Printf("[SPEAK] Waiting for AV Login request from camera...\n")
	}

	buf := make([]byte, 1024)
	c.speaker.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := c.speaker.Read(buf)
	if err != nil {
		return fmt.Errorf("read AV login: %w", err)
	}

	if c.verbose {
		fmt.Printf("[SPEAK] Received AV Login request: %d bytes\n", n)
	}

	if n < 24 {
		return fmt.Errorf("AV login too short: %d bytes", n)
	}

	checksum := binary.LittleEndian.Uint32(buf[20:])
	resp := c.buildAVLoginResponse(checksum)

	if c.verbose {
		fmt.Printf("[SPEAK] Sending AV Login response: %d bytes\n", len(resp))
	}

	if _, err = c.speaker.Write(resp); err != nil {
		return fmt.Errorf("write AV login response: %w", err)
	}

	// Camera may resend, respond again
	c.speaker.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if n, _ = c.speaker.Read(buf); n > 0 {
		if c.verbose {
			fmt.Printf("[SPEAK] Received AV Login resend: %d bytes\n", n)
		}
		c.speaker.Write(resp)
	}

	c.speaker.SetReadDeadline(time.Time{})

	if c.verbose {
		fmt.Printf("[SPEAK] AV Login complete, ready for audio\n")
	}

	return nil
}

func (c *Conn) buildDisco(stage byte) []byte {
	b := make([]byte, OldDiscoSize)
	copy(b, "\x04\x02\x1a\x02")                            // marker + mode
	binary.LittleEndian.PutUint16(b[4:], OldDiscoBodySize) // body size
	binary.LittleEndian.PutUint16(b[8:], CmdDiscoReq)      // 0x0601
	binary.LittleEndian.PutUint16(b[10:], 0x0021)          // flags
	body := b[OldHeaderSize:]
	copy(body[:UIDSize], c.uid)
	copy(body[36:], "\x01\x01\x02\x04") // unknown
	copy(body[40:], c.rid)
	body[48] = stage
	if stage == 1 && len(c.authKey) > 0 {
		copy(body[58:], c.authKey)
	}
	return b
}

func (c *Conn) buildNewDisco(seq, ticket uint16, isResponse bool) []byte {
	b := make([]byte, NewPacketSize)
	binary.LittleEndian.PutUint16(b[0:], MagicNewProto)  // 0xCC51
	binary.LittleEndian.PutUint16(b[4:], CmdNewDisco)    // 0x1002
	binary.LittleEndian.PutUint16(b[6:], NewPayloadSize) // 40 bytes
	if isResponse {
		binary.LittleEndian.PutUint16(b[8:], 0xFFFF) // response
	}
	binary.LittleEndian.PutUint16(b[12:], seq)
	binary.LittleEndian.PutUint16(b[14:], ticket)
	copy(b[16:24], c.sid)
	copy(b[24:32], "\x00\x08\x03\x04\x1d\x00\x00\x00") // SDK 4.3.8.0
	authKey := crypto.CalculateAuthKey(c.enr, c.mac)
	h := hmac.New(sha1.New, append([]byte(c.uid), authKey...))
	h.Write(b[:32])
	copy(b[32:52], h.Sum(nil))
	return b
}

func (c *Conn) buildNewKeepalive() []byte {
	c.kaSeq += 2
	b := make([]byte, NewKeepaliveSize)
	binary.LittleEndian.PutUint16(b[0:], MagicNewProto)   // 0xCC51
	binary.LittleEndian.PutUint16(b[4:], CmdNewKeepalive) // 0x1202
	binary.LittleEndian.PutUint16(b[6:], 0x0024)          // 36 bytes payload
	binary.LittleEndian.PutUint32(b[16:], c.kaSeq)        // counter
	copy(b[20:28], c.sid)                                 // session ID
	authKey := crypto.CalculateAuthKey(c.enr, c.mac)
	h := hmac.New(sha1.New, append([]byte(c.uid), authKey...))
	h.Write(b[:28])
	copy(b[28:48], h.Sum(nil))
	return b
}

func (c *Conn) buildSession() []byte {
	b := make([]byte, OldSessionSize)
	copy(b, "\x04\x02\x1a\x02")                          // marker + mode
	binary.LittleEndian.PutUint16(b[4:], OldSessionBody) // body size
	binary.LittleEndian.PutUint16(b[8:], CmdSessionReq)  // 0x0402
	binary.LittleEndian.PutUint16(b[10:], 0x0033)        // flags
	body := b[OldHeaderSize:]
	copy(body[:UIDSize], c.uid)
	copy(body[UIDSize:], c.rid)
	binary.LittleEndian.PutUint32(body[32:], uint32(time.Now().Unix()))
	return b
}

func (c *Conn) buildAVLoginPacket(magic uint16, size int, flags uint16, randomID []byte) []byte {
	b := make([]byte, size)
	binary.LittleEndian.PutUint16(b, magic)
	binary.LittleEndian.PutUint16(b[2:], ProtoVersion)
	binary.LittleEndian.PutUint16(b[16:], uint16(size-24)) // payload size
	binary.LittleEndian.PutUint16(b[18:], flags)
	copy(b[20:], randomID[:4])
	copy(b[24:], DefaultUser)                           // username
	copy(b[280:], c.enr)                                // password (ENR)
	binary.LittleEndian.PutUint32(b[540:], 2)           // security_mode=AV_SECURITY_AUTO
	binary.LittleEndian.PutUint32(b[552:], DefaultCaps) // capabilities
	return b
}

func (c *Conn) buildAVLoginResponse(checksum uint32) []byte {
	b := make([]byte, 60)
	binary.LittleEndian.PutUint16(b, 0x2100)        // magic
	binary.LittleEndian.PutUint16(b[2:], 0x000c)    // version
	b[4] = 0x10                                     // success
	binary.LittleEndian.PutUint32(b[16:], 0x24)     // payload size
	binary.LittleEndian.PutUint32(b[20:], checksum) // echo checksum
	b[29] = 0x01                                    // enable flag
	b[31] = 0x01                                    // two-way streaming
	binary.LittleEndian.PutUint32(b[36:], 0x04)     // buffer config
	binary.LittleEndian.PutUint32(b[40:], DefaultCaps)
	binary.LittleEndian.PutUint16(b[54:], 0x0003) // channel info
	binary.LittleEndian.PutUint16(b[56:], 0x0002)
	return b
}

func (c *Conn) buildAudioFrame(payload []byte, timestampUS uint32, codec uint16, sampleRate uint32, channels uint8) []byte {
	c.audioSeq++
	c.audioFrame++
	prevFrame := uint32(0)
	if c.audioFrame > 1 {
		prevFrame = c.audioFrame - 1
	}

	totalPayload := len(payload) + 16 // payload + frameinfo
	b := make([]byte, 36+totalPayload)

	// Outer header (36 bytes)
	b[0] = ChannelAudio      // 0x03
	b[1] = FrameTypeStartAlt // 0x09
	binary.LittleEndian.PutUint16(b[2:], ProtoVersion)
	binary.LittleEndian.PutUint32(b[4:], c.audioSeq)
	binary.LittleEndian.PutUint32(b[8:], timestampUS)
	if c.audioFrame == 1 {
		binary.LittleEndian.PutUint32(b[12:], 0x00000001)
	} else {
		binary.LittleEndian.PutUint32(b[12:], 0x00100001)
	}

	// Inner header
	b[16] = ChannelAudio
	b[17] = FrameTypeEndSingle
	binary.LittleEndian.PutUint16(b[18:], uint16(prevFrame))
	binary.LittleEndian.PutUint16(b[20:], 0x0001) // pkt_total
	binary.LittleEndian.PutUint16(b[22:], 0x0010) // flags
	binary.LittleEndian.PutUint32(b[24:], uint32(totalPayload))
	binary.LittleEndian.PutUint32(b[28:], prevFrame)
	binary.LittleEndian.PutUint32(b[32:], c.audioFrame)
	copy(b[36:], payload) // Payload + FrameInfo
	fi := b[36+len(payload):]
	binary.LittleEndian.PutUint16(fi, codec)
	fi[2] = BuildAudioFlags(sampleRate, true, channels == 2)
	fi[4] = 1 // online
	binary.LittleEndian.PutUint32(fi[12:], (c.audioFrame-1)*GetSamplesPerFrame(codec)*1000/sampleRate)
	return b
}

func (c *Conn) buildTxData(payload []byte, channel byte) []byte {
	bodySize := 12 + len(payload)
	b := make([]byte, 16+bodySize)
	copy(b, "\x04\x02\x1a\x0b")                            // marker + mode=data
	binary.LittleEndian.PutUint16(b[4:], uint16(bodySize)) // body size
	binary.LittleEndian.PutUint16(b[6:], c.seq)            // sequence
	c.seq++
	binary.LittleEndian.PutUint16(b[8:], CmdDataTX)   // 0x0407
	binary.LittleEndian.PutUint16(b[10:], 0x0021)     // flags
	copy(b[12:], c.rid[:2])                           // rid[0:2]
	b[14] = channel                                   // channel
	b[15] = 0x01                                      // marker
	binary.LittleEndian.PutUint32(b[16:], 0x0000000c) // const
	copy(b[20:], c.rid[:8])                           // rid
	copy(b[28:], payload)
	return b
}

func (c *Conn) buildNewTxData(payload []byte, channel byte) []byte {
	payloadSize := uint16(16 + len(payload) + NewAuthSize)
	b := make([]byte, NewHeaderSize+len(payload)+NewAuthSize)
	binary.LittleEndian.PutUint16(b[0:], MagicNewProto) // 0xCC51
	binary.LittleEndian.PutUint16(b[4:], CmdNewDTLS)    // 0x1502
	binary.LittleEndian.PutUint16(b[6:], payloadSize)
	binary.LittleEndian.PutUint16(b[12:], uint16(0x0010)|(uint16(channel)<<8)) // channel in high byte
	binary.LittleEndian.PutUint16(b[14:], c.ticket)
	copy(b[16:24], c.sid)
	binary.LittleEndian.PutUint32(b[24:], 1) // const
	copy(b[NewHeaderSize:], payload)
	authKey := crypto.CalculateAuthKey(c.enr, c.mac)
	h := hmac.New(sha1.New, append([]byte(c.uid), authKey...))
	h.Write(b[:NewHeaderSize])
	copy(b[NewHeaderSize+len(payload):], h.Sum(nil))
	return b
}

func (c *Conn) buildACK() []byte {
	if c.ackFlags == 0 {
		c.ackFlags = 0x0001
	} else if c.ackFlags < 0x0007 {
		c.ackFlags++
	}
	b := make([]byte, 24)
	binary.LittleEndian.PutUint16(b[0:], MagicACK)     // 0x0009
	binary.LittleEndian.PutUint16(b[2:], ProtoVersion) // 0x000c
	binary.LittleEndian.PutUint32(b[4:], c.avSeq)      // tx seq
	c.avSeq++
	binary.LittleEndian.PutUint32(b[8:], 0xffffffff)              // rx seq
	binary.LittleEndian.PutUint16(b[12:], c.ackFlags)             // ack flags
	binary.LittleEndian.PutUint32(b[16:], uint32(c.ackFlags)<<16) // ack counter
	return b
}

func (c *Conn) buildKeepAlive(incoming []byte) []byte {
	b := make([]byte, 24)
	copy(b, "\x04\x02\x1a\x0a")                           // marker + mode
	binary.LittleEndian.PutUint16(b[4:], 8)               // body size
	binary.LittleEndian.PutUint16(b[8:], CmdKeepaliveReq) // 0x0427
	binary.LittleEndian.PutUint16(b[10:], 0x0021)         // flags
	if len(incoming) >= 8 {
		copy(b[16:], incoming[:8]) // echo payload
	}
	return b
}

func (c *Conn) buildIOCtrlFrame(payload []byte) []byte {
	b := make([]byte, 40+len(payload))
	binary.LittleEndian.PutUint16(b, ProtoVersion)     // magic
	binary.LittleEndian.PutUint16(b[2:], ProtoVersion) // version
	binary.LittleEndian.PutUint32(b[4:], c.avSeq)      // av seq
	c.avSeq++
	binary.LittleEndian.PutUint16(b[16:], MagicIOCtrl)            // 0x7000
	binary.LittleEndian.PutUint16(b[18:], c.seqCmd)               // sub channel
	binary.LittleEndian.PutUint32(b[20:], 1)                      // ioctl seq
	binary.LittleEndian.PutUint32(b[24:], uint32(len(payload)+4)) // payload size
	binary.LittleEndian.PutUint32(b[28:], uint32(c.seqCmd))       // flag
	b[37] = 0x01
	copy(b[40:], payload)
	c.seqCmd++
	return b
}

func derivePSK(enr string) []byte {
	// TUTK SDK treats the PSK as a NULL-terminated C string, so if SHA256(ENR)
	// contains a 0x00 byte, the PSK is truncated at that position.
	// This matches iOS Wyze app behavior discovered via Frida instrumentation.

	hash := sha256.Sum256([]byte(enr))

	pskLen := 32
	for i := range 32 {
		if hash[i] == 0x00 {
			pskLen = i
			break
		}
	}

	// bytes up to first 0x00, rest padded with zeros
	psk := make([]byte, 32)
	copy(psk[:pskLen], hash[:pskLen])
	return psk
}

func genRandomID() []byte {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return b
}
