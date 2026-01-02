package tutk

import (
	"context"
	"crypto/rand"
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
	PSKIdentity    = "AUTHPWD_admin"
	DefaultUser    = "admin"
	DefaultPort    = 32761           // TUTK discovery port
	MaxPacketSize  = 2048            // Max single packet size
	ReadBufferSize = 2 * 1024 * 1024 // 2MB for video streams

	DiscoTimeout     = 5000 * time.Millisecond // Total timeout for discovery
	DiscoInterval    = 100 * time.Millisecond  // Interval between discovery packets
	SessionTimeout   = 5000 * time.Millisecond // Total timeout for session setup
	ReadWaitInterval = 50 * time.Millisecond   // Read wait interval per iteration
)

type FrameAssembler struct {
	frameNo   uint32
	pktTotal  uint16
	packets   map[uint16][]byte // pkt_idx -> payload
	frameInfo *FrameInfo
}

type Conn struct {
	udpConn        *net.UDPConn
	addr           *net.UDPAddr
	broadcastAddrs []*net.UDPAddr
	randomID       []byte
	uid            string
	authKey        string
	enr            string
	psk            []byte
	iotcTxSeq      uint16
	avLoginResp    *AVLoginResponse

	// DTLS - Main Channel (we = Client)
	mainConn *dtls.Conn
	mainBuf  chan []byte

	// DTLS - Speaker Channel (we = Server)
	speakerConn *dtls.Conn
	speakerBuf  chan []byte

	ioctrl      chan []byte
	ackReceived chan struct{}
	errors      chan error

	frameAssemblers map[byte]*FrameAssembler // channel -> assembler
	packetQueue     chan *Packet

	avTxSeq   uint32
	ioctrlSeq uint16

	// Audio TX state (for intercom)
	audioTxSeq     uint32
	audioTxFrameNo uint32

	lastAckCounter uint16
	ackFlags       uint16

	baseTS uint64

	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.RWMutex
	done    chan struct{}
	verbose bool
}

func Dial(host, uid, authKey, enr string, verbose bool) (*Conn, error) {
	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	_ = conn.SetReadBuffer(ReadBufferSize)

	ctx, cancel := context.WithCancel(context.Background())

	hash := sha256.Sum256([]byte(enr))
	psk := hash[:]

	c := &Conn{
		udpConn:        conn,
		addr:           &net.UDPAddr{IP: net.ParseIP(host), Port: DefaultPort},
		broadcastAddrs: getBroadcastAddrs(DefaultPort, verbose),
		randomID:       genRandomID(),
		uid:            uid,
		authKey:        authKey,
		enr:            enr,
		psk:            psk,
		verbose:        verbose,
		ctx:            ctx,
		cancel:         cancel,
		mainBuf:    make(chan []byte, 64),
		speakerBuf: make(chan []byte, 64),
		packetQueue: make(chan *Packet, 128),
		done:        make(chan struct{}),
		ioctrl:      make(chan []byte, 16),
		ackReceived: make(chan struct{}, 1),
		errors:      make(chan error, 1),
	}

	if err = c.discovery(); err != nil {
		_ = c.Close()
		return nil, err
	}

	// Start IOTC reader goroutine for DTLS routing
	c.wg.Add(1)
	go c.iotcReader()

	// Perform DTLS client handshake on Main channel
	if err = c.connect(); err != nil {
		_ = c.Close()
		return nil, err
	}

	// Start AV data worker
	c.wg.Add(1)
	go c.worker()

	return c, nil
}

func (c *Conn) AVClientStart(timeout time.Duration) error {
	randomID := genRandomID()
	pkt1 := c.buildAVLoginPacket(MagicAVLogin1, 570, 0x0001, randomID)
	pkt2 := c.buildAVLoginPacket(MagicAVLogin2, 572, 0x0000, randomID)
	pkt2[20]++ // pkt2 has randomID incremented by 1

	if _, err := c.mainConn.Write(pkt1); err != nil {
		return fmt.Errorf("AV login 1 failed: %w", err)
	}

	time.Sleep(50 * time.Millisecond)

	if _, err := c.mainConn.Write(pkt2); err != nil {
		return fmt.Errorf("AV login 2 failed: %w", err)
	}

	// Wait for response
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return context.DeadlineExceeded
		}

		select {
		case data, ok := <-c.ioctrl:
			if !ok {
				return io.EOF
			}
			if len(data) >= 32 && binary.LittleEndian.Uint16(data[0:2]) == MagicAVLoginResp {
				// Parse response inline
				c.avLoginResp = &AVLoginResponse{
					ServerType:      binary.LittleEndian.Uint32(data[4:8]),
					Resend:          int32(data[29]),
					TwoWayStreaming: int32(data[31]),
				}

				if c.verbose {
					fmt.Printf("[TUTK] AV Login Response: two_way_streaming=%d\n", c.avLoginResp.TwoWayStreaming)
				}

				_ = c.sendACK()
				return nil
			}
		case <-c.ctx.Done():
			return c.ctx.Err()
		}
	}
}

func (c *Conn) AVServStart() error {
	if c.verbose {
		fmt.Printf("[DTLS] Waiting for client handshake on channel %d\n", IOTCChannelBack)
		fmt.Printf("[DTLS] PSK Identity: %s\n", PSKIdentity)
		fmt.Printf("[DTLS] PSK Key: %s\n", hex.EncodeToString(c.psk))
	}

	config := c.buildDTLSConfig(true)

	// Create adapter for speaker channel
	adapter := &ChannelAdapter{
		conn:    c,
		channel: IOTCChannelBack,
	}

	conn, err := dtls.Server(adapter, c.addr, config)
	if err != nil {
		return fmt.Errorf("dtls: server handshake failed: %w", err)
	}

	c.mu.Lock()
	c.speakerConn = conn
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
	c.audioTxSeq = 0
	c.audioTxFrameNo = 0

	if c.speakerConn != nil {
		err := c.speakerConn.Close()
		c.speakerConn = nil
		return err
	}
	return nil
}

func (c *Conn) AVRecvFrameData() (*Packet, error) {
	select {
	case pkt, ok := <-c.packetQueue:
		if !ok {
			return nil, io.EOF
		}
		return pkt, nil
	case err := <-c.errors:
		return nil, err
	case <-c.done:
		return nil, io.EOF
	case <-c.ctx.Done():
		return nil, io.EOF
	}
}

func (c *Conn) AVSendAudioData(codec uint16, payload []byte, timestampUS uint32, sampleRate uint32, channels uint8) error {
	c.mu.Lock()
	conn := c.speakerConn
	if conn == nil {
		c.mu.Unlock()
		return fmt.Errorf("speaker channel not connected")
	}

	// Build frame with 36-byte header + audio + 16-byte FrameInfo (FrameInfo inside payload!)
	frame := c.buildAudioFrame(payload, timestampUS, codec, sampleRate, channels)

	if c.verbose {
		c.logAudioTX(frame, codec, len(payload), timestampUS, sampleRate, channels)
	}
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

func (c *Conn) SendIOCtrl(cmdID uint16, payload []byte) error {
	frame := c.buildIOCtrlFrame(payload)
	if _, err := c.mainConn.Write(frame); err != nil {
		return err
	}

	// Block until ACK received (like SDK)
	select {
	case <-c.ackReceived:
		if c.verbose {
			fmt.Printf("[Conn] SendIOCtrl K%d: ACK received\n", cmdID)
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("ACK timeout for K%d", cmdID)
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *Conn) RecvIOCtrl(timeout time.Duration) (cmdID uint16, data []byte, err error) {
	select {
	case data, ok := <-c.ioctrl:
		if !ok {
			return 0, nil, io.EOF
		}
		// Parse cmdID from HL header at offset 4-5
		if len(data) >= 6 {
			cmdID = binary.LittleEndian.Uint16(data[4:6])
		}
		// Send ACK after receiving
		_ = c.sendACK()
		if c.verbose {
			fmt.Printf("[Conn] RecvIOCtrl: received K%d (%d bytes)\n", cmdID, len(data))
		}
		return cmdID, data, nil
	case <-time.After(timeout):
		return 0, nil, context.DeadlineExceeded
	case <-c.ctx.Done():
		return 0, nil, c.ctx.Err()
	}
}

func (c *Conn) GetAVLoginResponse() *AVLoginResponse {
	return c.avLoginResp
}

func (c *Conn) IsBackchannelReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.speakerConn != nil
}

func (c *Conn) RemoteAddr() *net.UDPAddr {
	return c.addr
}

func (c *Conn) LocalAddr() *net.UDPAddr {
	return c.udpConn.LocalAddr().(*net.UDPAddr)
}

func (c *Conn) SetDeadline(t time.Time) error {
	return c.udpConn.SetDeadline(t)
}

func (c *Conn) Close() error {
	// Signal done to stop goroutines
	select {
	case <-c.done:
	default:
		close(c.done)
	}

	// Close DTLS connections
	c.mu.Lock()
	if c.mainConn != nil {
		c.mainConn.Close()
		c.mainConn = nil
	}
	if c.speakerConn != nil {
		c.speakerConn.Close()
		c.speakerConn = nil
	}
	c.mu.Unlock()

	c.cancel()

	// Wait for goroutines
	c.wg.Wait()

	close(c.ioctrl)
	close(c.errors)

	return c.udpConn.Close()
}

func (c *Conn) discovery() error {
	_ = c.udpConn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := c.discoStage1(); err != nil {
		return fmt.Errorf("disco stage 1: %w", err)
	}

	c.discoStage2()

	if err := c.sessionSetup(); err != nil {
		return fmt.Errorf("session setup: %w", err)
	}

	_ = c.udpConn.SetDeadline(time.Time{})
	return nil
}

func (c *Conn) discoStage1() error {
	pkt := c.buildDisco(1)
	encrypted := crypto.TransCodeBlob(pkt)

	if c.verbose {
		fmt.Printf("[IOTC] Disco Stage 1: timeout=%v interval=%v broadcasts=%d\n",
			DiscoTimeout, DiscoInterval, len(c.broadcastAddrs))
	}

	deadline := time.Now().Add(DiscoTimeout)
	lastSend := time.Time{}
	buf := make([]byte, MaxPacketSize)

	for time.Now().Before(deadline) {
		if time.Since(lastSend) >= DiscoInterval {
			for _, bcast := range c.broadcastAddrs {
				c.udpConn.WriteToUDP(encrypted, bcast)
				if c.verbose {
					fmt.Printf("[IOTC] Disco Stage 1: sent to %s\n", bcast)
				}
			}
			lastSend = time.Now()
		}

		c.udpConn.SetReadDeadline(time.Now().Add(ReadWaitInterval))
		n, addr, err := c.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return err
		}

		data := crypto.ReverseTransCodeBlob(buf[:n])
		if len(data) < 16 {
			continue
		}

		cmd := binary.LittleEndian.Uint16(data[8:10])
		if c.verbose {
			fmt.Printf("[IOTC] Disco Stage 1: received cmd=0x%04x from %s\n", cmd, addr)
		}

		if cmd == CmdDiscoRes {
			c.addr = addr
			if c.verbose {
				fmt.Printf("[IOTC] Disco Stage 1: success! Camera at %s\n", addr)
			}
			return nil
		}
	}

	return fmt.Errorf("timeout after %v", DiscoTimeout)
}

func (c *Conn) discoStage2() {
	pkt := c.buildDisco(2)
	encrypted := crypto.TransCodeBlob(pkt)
	_, _ = c.udpConn.WriteToUDP(encrypted, c.addr)
	time.Sleep(100 * time.Millisecond)
}

func (c *Conn) sessionSetup() error {
	pkt := c.buildSession()

	if c.verbose {
		fmt.Printf("[IOTC] Session setup: target=%s\n", c.addr)
	}

	// Send request
	if _, err := c.sendEncrypted(pkt); err != nil {
		return err
	}

	// Wait for response
	buf := make([]byte, MaxPacketSize)
	c.udpConn.SetReadDeadline(time.Now().Add(SessionTimeout))

	for {
		n, addr, err := c.udpConn.ReadFromUDP(buf)
		if err != nil {
			return fmt.Errorf("timeout: %w", err)
		}

		data := crypto.ReverseTransCodeBlob(buf[:n])
		if len(data) < 16 {
			continue
		}

		cmd := binary.LittleEndian.Uint16(data[8:10])
		if c.verbose {
			fmt.Printf("[IOTC] Session setup: received cmd=0x%04x from %s\n", cmd, addr)
		}

		if cmd == CmdSessionRes {
			c.addr = addr
			if c.verbose {
				fmt.Printf("[IOTC] Session setup: success!\n")
			}
			return nil
		}
	}
}

func (c *Conn) connect() error {
	if c.verbose {
		fmt.Printf("[DTLS] Starting client handshake on channel %d\n", IOTCChannelMain)
		fmt.Printf("[DTLS] PSK Identity: %s\n", PSKIdentity)
		fmt.Printf("[DTLS] PSK Key: %s\n", hex.EncodeToString(c.psk))
	}

	config := c.buildDTLSConfig(false)

	// Create adapter for main channel
	adapter := &ChannelAdapter{
		conn:    c,
		channel: IOTCChannelMain,
	}

	conn, err := dtls.Client(adapter, c.addr, config)
	if err != nil {
		return fmt.Errorf("dtls: client handshake failed: %w", err)
	}

	c.mu.Lock()
	c.mainConn = conn
	c.mu.Unlock()

	if c.verbose {
		fmt.Printf("[DTLS] Client handshake complete on channel %d\n", IOTCChannelMain)
	}

	return nil
}

func (c *Conn) iotcReader() {
	defer c.wg.Done()

	buf := make([]byte, MaxPacketSize)

	for {
		select {
		case <-c.done:
			return
		default:
		}

		// Inline receive with timeout
		c.udpConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := c.udpConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		data := crypto.ReverseTransCodeBlob(buf[:n])
		if addr.Port != c.addr.Port || !addr.IP.Equal(c.addr.IP) {
			c.addr = addr
		}

		if len(data) < 16 {
			continue
		}

		cmd := binary.LittleEndian.Uint16(data[8:10])

		if cmd == CmdKeepaliveRes && len(data) > 16 {
			payload := data[16:]
			if len(payload) >= 8 {
				keepaliveResp := c.buildKeepaliveResponse(payload)
				_, _ = c.sendEncrypted(keepaliveResp)
				if c.verbose {
					fmt.Printf("[DTLS] Keepalive response sent\n")
				}
			}
			continue
		}

		if cmd == CmdDataRX && len(data) > 28 {
			// Debug: Dump IOTC header to verify structure
			if c.verbose && len(data) >= 32 {
				fmt.Printf("[IOTC] RX Header dump (32 bytes):\n")
				fmt.Printf("  [0-7]:   %02x %02x %02x %02x  %02x %02x %02x %02x\n",
					data[0], data[1], data[2], data[3], data[4], data[5], data[6], data[7])
				fmt.Printf("  [8-15]:  %02x %02x %02x %02x  %02x %02x %02x %02x  (cmd@8-9, ch@14)\n",
					data[8], data[9], data[10], data[11], data[12], data[13], data[14], data[15])
				fmt.Printf("  [16-23]: %02x %02x %02x %02x  %02x %02x %02x %02x\n",
					data[16], data[17], data[18], data[19], data[20], data[21], data[22], data[23])
				fmt.Printf("  [24-31]: %02x %02x %02x %02x  %02x %02x %02x %02x  (dtls starts @28)\n",
					data[24], data[25], data[26], data[27], data[28], data[29], data[30], data[31])
			}

			dtlsPayload := data[28:]

			// Channel byte is at position 14 in IOTC header
			channel := data[14]

			if c.verbose {
				fmt.Printf("[IOTC] RX cmd=0x%04x len=%d ch=%d dtlsLen=%d\n", cmd, len(data), channel, len(dtlsPayload))
				if len(dtlsPayload) >= 13 {
					contentType := dtlsPayload[0]
					fmt.Printf("[DTLS] ch=%d contentType=%d first8=[%02x %02x %02x %02x %02x %02x %02x %02x]\n",
						channel, contentType, dtlsPayload[0], dtlsPayload[1], dtlsPayload[2], dtlsPayload[3],
						dtlsPayload[4], dtlsPayload[5], dtlsPayload[6], dtlsPayload[7])
				}
			}

			// Copy data since buffer is reused
			dataCopy := make([]byte, len(dtlsPayload))
			copy(dataCopy, dtlsPayload)

			// Route based on channel
			var buf chan []byte
			switch channel {
			case IOTCChannelMain:
				buf = c.mainBuf
			case IOTCChannelBack:
				buf = c.speakerBuf
			}

			if buf != nil {
				select {
				case buf <- dataCopy:
				default:
					// Drop oldest if full
					select {
					case <-buf:
					default:
					}
					buf <- dataCopy
				}
			}
		}
	}
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

		n, err := c.mainConn.Read(buf)
		if err != nil {
			select {
			case c.errors <- err:
			default:
			}
			return
		}

		if n < 2 {
			continue
		}

		// Debug: dump first bytes to see what we actually receive
		if c.verbose && n >= 36 {
			fmt.Printf("[Conn] worker raw: n=%d\n", n)
			fmt.Printf("[Conn]   first16: %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x\n",
				buf[0], buf[1], buf[2], buf[3], buf[4], buf[5], buf[6], buf[7],
				buf[8], buf[9], buf[10], buf[11], buf[12], buf[13], buf[14], buf[15])
			fmt.Printf("[Conn]   off16-31: %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x %02x\n",
				buf[16], buf[17], buf[18], buf[19], buf[20], buf[21], buf[22], buf[23],
				buf[24], buf[25], buf[26], buf[27], buf[28], buf[29], buf[30], buf[31])
		} else if c.verbose && n >= 8 {
			fmt.Printf("[Conn] worker raw: n=%d first8=[%02x %02x %02x %02x %02x %02x %02x %02x]\n",
				n, buf[0], buf[1], buf[2], buf[3], buf[4], buf[5], buf[6], buf[7])
		}

		c.route(buf[:n])
	}
}

func (c *Conn) route(data []byte) {
	//	[channel][frameType][version_lo][version_hi][seq_lo][seq_hi]...
	//	channel: 0x03=Audio, 0x05=I-Video, 0x07=P-Video
	//	frameType: 0x00=cont, 0x05=end, 0x08=I-start, 0x0d=end-44

	if len(data) < 2 {
		return
	}

	// Check for control frame magic values first (uint16 LE)
	magic := binary.LittleEndian.Uint16(data[0:2])

	switch magic {
	case MagicAVLoginResp:
		// AV Login Response - send full data for parsing
		c.queueIOCtrlData(data)
		return

	case MagicIOCtrl:
		// IOCTRL Response Frame (K10001, K10003)
		if len(data) >= 32 {
			for i := 32; i+2 < len(data); i++ {
				if data[i] == 'H' && data[i+1] == 'L' {
					c.queueIOCtrlData(data[i:])
					return
				}
			}
		}
		return

	case MagicChannelMsg:
		// Channel message
		if len(data) >= 36 {
			opCode := data[16]
			if opCode == 0x00 {
				for i := 36; i+2 < len(data); i++ {
					if data[i] == 'H' && data[i+1] == 'L' {
						c.queueIOCtrlData(data[i:])
						return
					}
				}
			}
		}
		return

	case MagicACK:
		// ACK from camera
		select {
		case c.ackReceived <- struct{}{}:
		default:
		}
		return
	}

	// Check for AV Data packet (channel byte at offset 0)
	channel := data[0]
	if channel == ChannelAudio || channel == ChannelIVideo || channel == ChannelPVideo {
		c.handleAVData(data)
		return
	}

	// Unknown packet type
	if c.verbose {
		fmt.Printf("[Conn] Unknown frame: type=0x%02x len=%d\n", data[0], len(data))
	}
}

func (c *Conn) handleSpeakerAVLogin() error {
	// Read AV Login request from camera (SDK receives 570 bytes)
	buf := make([]byte, 1024)
	c.speakerConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := c.speakerConn.Read(buf)
	if err != nil {
		return fmt.Errorf("read AV login: %w", err)
	}

	if c.verbose {
		fmt.Printf("[SPEAK] Received AV Login request: %d bytes\n", n)
	}

	// Need at least 24 bytes to read the checksum
	if n < 24 {
		return fmt.Errorf("AV login too short: %d bytes", n)
	}

	// Extract checksum from incoming request (bytes 20-23) - MUST echo this back!
	checksum := binary.LittleEndian.Uint32(buf[20:24])

	// Build AV Login response (60 bytes like SDK)
	resp := c.buildAVLoginResponse(checksum)

	if c.verbose {
		fmt.Printf("[SPEAK] Sending AV Login response: %d bytes\n", len(resp))
	}

	_, err = c.speakerConn.Write(resp)
	if err != nil {
		return fmt.Errorf("write AV login response: %w", err)
	}

	// Camera will resend AV-Login, respond again with AV-LoginResp
	c.speakerConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _ = c.speakerConn.Read(buf)
	if n > 0 {
		if c.verbose {
			fmt.Printf("[SPEAK] Received AV Login resend: %d bytes\n", n)
		}
		// Send second AV-LoginResp
		if c.verbose {
			fmt.Printf("[SPEAK] Sending second AV Login response: %d bytes\n", len(resp))
		}
		c.speakerConn.Write(resp)
	}

	// Clear deadline
	c.speakerConn.SetReadDeadline(time.Time{})

	if c.verbose {
		fmt.Printf("[SPEAK] AV Login complete, ready for audio\n")
	}

	return nil
}

func (c *Conn) handleAVData(data []byte) {
	// Parse packet header to get pkt_idx, pkt_total, frame_no
	hdr := ParsePacketHeader(data)
	if hdr == nil {
		fmt.Printf("[Conn] Invalid AV packet header, len=%d\n", len(data))
		return
	}

	// Debug: Log raw Wire-Header bytes
	if c.verbose {
		fmt.Printf("[WIRE] ch=0x%02x type=0x%02x len=%d pkt=%d/%d frame=%d\n",
			hdr.Channel, hdr.FrameType, len(data), hdr.PktIdx, hdr.PktTotal, hdr.FrameNo)
		fmt.Printf("       RAW[0..35]: ")
		for i := 0; i < 36 && i < len(data); i++ {
			fmt.Printf("%02x ", data[i])
		}
		fmt.Printf("\n")
	}

	// Extract payload and try to detect FRAMEINFO
	payload, fi := c.extractPayload(data, hdr.Channel)
	if payload == nil {
		return
	}

	if c.verbose {
		c.logAVPacket(hdr.Channel, hdr.FrameType, payload, fi)
	}

	// Route to handler
	switch hdr.Channel {
	case ChannelAudio:
		c.handleAudio(payload, fi)
	case ChannelIVideo, ChannelPVideo:
		c.handleVideo(hdr.Channel, hdr, payload, fi)
	}
}

func (c *Conn) extractPayload(data []byte, channel byte) ([]byte, *FrameInfo) {
	if len(data) < 2 {
		return nil, nil
	}

	frameType := data[1]

	// Determine header size and FrameInfo size based on frameType
	headerSize := 28
	frameInfoSize := 0 // 0 means no FrameInfo

	switch frameType {
	case FrameTypeStart:
		// Extended start packet - 36-byte header, no FrameInfo
		headerSize = 36
	case FrameTypeStartAlt:
		// StartAlt - 36-byte header
		// Has FrameInfo only if pkt_total == 1 (single-packet frame)
		headerSize = 36
		if len(data) >= 22 {
			pktTotal := uint16(data[20]) | uint16(data[21])<<8
			if pktTotal == 1 {
				frameInfoSize = FrameInfoSize
			}
		}
	case FrameTypeCont, FrameTypeContAlt:
		// Continuation packet - standard 28-byte header, no FrameInfo
		headerSize = 28
	case FrameTypeEndSingle, FrameTypeEndMulti:
		// End packet - standard 28-byte header, 40-byte FrameInfo
		headerSize = 28
		frameInfoSize = FrameInfoSize
	case FrameTypeEndExt:
		// Extended end packet - 36-byte header, 40-byte FrameInfo
		headerSize = 36
		frameInfoSize = FrameInfoSize
	default:
		// Unknown frame type - use 28-byte header as fallback (most common)
		headerSize = 28
	}

	if len(data) < headerSize {
		return nil, nil
	}

	// If this packet type doesn't have FrameInfo, return payload without it
	if frameInfoSize == 0 {
		return data[headerSize:], nil
	}

	// End packets have FrameInfo - validate size
	if len(data) < headerSize+frameInfoSize {
		return data[headerSize:], nil
	}

	fi := ParseFrameInfo(data)

	// Validate codec matches channel type
	validCodec := false
	switch channel {
	case ChannelIVideo, ChannelPVideo:
		validCodec = IsVideoCodec(fi.CodecID)
	case ChannelAudio:
		validCodec = IsAudioCodec(fi.CodecID)
	}

	if validCodec {
		if c.verbose {
			fiRaw := data[len(data)-frameInfoSize:]
			fmt.Printf("[FRAMEINFO RAW %d bytes]:\n", frameInfoSize)
			fmt.Printf("       [0-15]:  ")
			for i := 0; i < 16 && i < len(fiRaw); i++ {
				fmt.Printf("%02x ", fiRaw[i])
			}
			fmt.Printf("\n       [16-31]: ")
			for i := 16; i < 32 && i < len(fiRaw); i++ {
				fmt.Printf("%02x ", fiRaw[i])
			}
			fmt.Printf("\n       [32-%d]: ", frameInfoSize-1)
			for i := 32; i < frameInfoSize && i < len(fiRaw); i++ {
				fmt.Printf("%02x ", fiRaw[i])
			}
			fmt.Printf("\n")
		}

		payload := data[headerSize : len(data)-frameInfoSize]
		return payload, fi
	}

	return data[headerSize:], nil
}

func (c *Conn) handleVideo(channel byte, hdr *PacketHeader, payload []byte, fi *FrameInfo) {
	if c.frameAssemblers == nil {
		c.frameAssemblers = make(map[byte]*FrameAssembler)
	}

	asm := c.frameAssemblers[channel]

	// Frame transition detection: new frame number = previous frame complete
	if asm != nil && hdr.FrameNo != asm.frameNo {
		gotAll := uint16(len(asm.packets)) == asm.pktTotal

		if gotAll && asm.frameInfo != nil {
			// Perfect: all packets + FrameInfo present
			c.assembleAndQueueVideo(channel, asm)
		} else if c.verbose {
			// Debugging: what exactly is missing?
			if gotAll && asm.frameInfo == nil {
				fmt.Printf("[VIDEO] Frame #%d: all %d packets received but End packet lost (no FrameInfo)\n",
					asm.frameNo, asm.pktTotal)
			} else {
				fmt.Printf("[VIDEO] Frame #%d: incomplete %d/%d packets\n",
					asm.frameNo, len(asm.packets), asm.pktTotal)
			}
		}
		asm = nil
	}

	// Create new assembler if needed
	if asm == nil {
		asm = &FrameAssembler{
			frameNo:  hdr.FrameNo,
			pktTotal: hdr.PktTotal,
			packets:  make(map[uint16][]byte, hdr.PktTotal),
		}
		c.frameAssemblers[channel] = asm
	}

	// Store packet (with pkt_idx as key!)
	// IMPORTANT: Always register the packet, even if payload is empty!
	// End packets may have 0 bytes payload (all data in previous packets)
	// but still need to be counted for completeness check.
	// CRITICAL: Must copy payload! The underlying buffer is reused by the worker.
	payloadCopy := make([]byte, len(payload))
	copy(payloadCopy, payload)
	asm.packets[hdr.PktIdx] = payloadCopy

	// Store FrameInfo if present
	if fi != nil {
		asm.frameInfo = fi
	}

	// Check if frame is complete
	if uint16(len(asm.packets)) == asm.pktTotal && asm.frameInfo != nil {
		c.assembleAndQueueVideo(channel, asm)
		delete(c.frameAssemblers, channel)
	}
}

func (c *Conn) assembleAndQueueVideo(channel byte, asm *FrameAssembler) {
	fi := asm.frameInfo

	// Assemble packets in correct order
	var payload []byte
	for i := uint16(0); i < asm.pktTotal; i++ {
		if pkt, ok := asm.packets[i]; ok {
			payload = append(payload, pkt...)
		}
	}

	// Size validation
	if fi.PayloadSize > 0 && len(payload) != int(fi.PayloadSize) {
		if c.verbose {
			fmt.Printf("[VIDEO] Frame #%d size mismatch: got=%d expected=%d, discarding\n",
				asm.frameNo, len(payload), fi.PayloadSize)
		}
		return
	}

	if len(payload) == 0 {
		return
	}

	// Calculate RTP timestamp (90kHz for video) using relative timestamps
	// to avoid uint64 overflow (absoluteTS * clockRate exceeds uint64 max)
	absoluteTS := uint64(fi.Timestamp)*1000000 + uint64(fi.TimestampUS)
	if c.baseTS == 0 {
		c.baseTS = absoluteTS
	}
	relativeUS := absoluteTS - c.baseTS
	const clockRate uint64 = 90000
	rtpTS := uint32(relativeUS * clockRate / 1000000)

	pkt := &Packet{
		Channel:    channel,
		Payload:    payload,
		Codec:      fi.CodecID,
		Timestamp:  rtpTS,
		IsKeyframe: fi.IsKeyframe(),
		FrameNo:    fi.FrameNo,
	}

	if c.verbose {
		frameType := "P"
		if fi.IsKeyframe() {
			frameType = "I"
		}
		fmt.Printf("[VIDEO] #%d %s %s size=%d rtp=%d\n",
			fi.FrameNo, CodecName(fi.CodecID), frameType, len(payload), rtpTS)
	}

	c.queuePacket(pkt)
}

func (c *Conn) handleAudio(payload []byte, fi *FrameInfo) {
	if len(payload) == 0 || fi == nil {
		return
	}

	var sampleRate uint32
	var channels uint8

	// Parse ADTS for AAC codecs, use FRAMEINFO for others
	switch fi.CodecID {
	case AudioCodecAACRaw, AudioCodecAACADTS, AudioCodecAACLATM, AudioCodecAACWyze:
		sampleRate, channels = ParseAudioParams(payload, fi)
	default:
		sampleRate = fi.SampleRate()
		channels = fi.Channels()
	}

	// Calculate RTP timestamp using relative timestamps to avoid uint64 overflow
	// Uses shared baseTS with video for proper A/V sync
	absoluteTS := uint64(fi.Timestamp)*1000000 + uint64(fi.TimestampUS)
	if c.baseTS == 0 {
		c.baseTS = absoluteTS
	}
	relativeUS := absoluteTS - c.baseTS
	clockRate := uint64(sampleRate)
	rtpTS := uint32(relativeUS * clockRate / 1000000)

	pkt := &Packet{
		Channel:    ChannelAudio,
		Payload:    payload,
		Codec:      fi.CodecID,
		Timestamp:  rtpTS,
		SampleRate: sampleRate,
		Channels:   channels,
		FrameNo:    fi.FrameNo,
	}

	if c.verbose {
		fmt.Printf("[AUDIO] #%d %s size=%d rate=%d ch=%d rtp=%d\n",
			fi.FrameNo, AudioCodecName(fi.CodecID), len(payload), sampleRate, channels, rtpTS)
	}

	c.queuePacket(pkt)
}

func (c *Conn) queuePacket(pkt *Packet) {
	select {
	case c.packetQueue <- pkt:
	default:
		// Queue full - drop oldest
		select {
		case <-c.packetQueue:
		default:
		}
		c.packetQueue <- pkt
	}
}

func (c *Conn) queueIOCtrlData(data []byte) {
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	select {
	case c.ioctrl <- dataCopy:
	default:
		select {
		case <-c.ioctrl:
		default:
		}
		c.ioctrl <- dataCopy
	}
}

func (c *Conn) sendACK() error {
	ack := c.buildACK()

	if c.verbose {
		fmt.Printf("[Conn] SendACK: txSeq=%d flags=0x%04x\n", c.avTxSeq-1, c.ackFlags)
	}

	_, err := c.mainConn.Write(ack)
	return err
}

func (c *Conn) sendIOTC(payload []byte, channel byte) (int, error) {
	frame := c.buildDataTXChannel(payload, channel)
	return c.sendEncrypted(frame)
}

func (c *Conn) sendEncrypted(data []byte) (int, error) {
	encrypted := crypto.TransCodeBlob(data)
	return c.udpConn.WriteToUDP(encrypted, c.addr)
}

func (c *Conn) buildAudioFrame(payload []byte, timestampUS uint32, codec uint16, sampleRate uint32, channels uint8) []byte {
	const frameInfoSize = 16
	const headerSize = 36

	c.audioTxSeq++
	c.audioTxFrameNo++

	totalPayload := len(payload) + frameInfoSize
	frame := make([]byte, headerSize+totalPayload)

	// Calculate prev_frame_no (0 for first frame, otherwise frame_no - 1)
	prevFrameNo := uint32(0)
	if c.audioTxFrameNo > 1 {
		prevFrameNo = c.audioTxFrameNo - 1
	}

	// Type 0x09 "Single" - 36-byte header with full timestamp
	frame[0] = ChannelAudio                                    // 0x03
	frame[1] = FrameTypeStartAlt                               // 0x09
	binary.LittleEndian.PutUint16(frame[2:4], ProtocolVersion) // 0x000c

	binary.LittleEndian.PutUint32(frame[4:8], c.audioTxSeq)
	binary.LittleEndian.PutUint32(frame[8:12], timestampUS) // Timestamp in header

	// Flags at [12-15]: first frame uses 0x00000001, subsequent use 0x00100001
	if c.audioTxFrameNo == 1 {
		binary.LittleEndian.PutUint32(frame[12:16], 0x00000001)
	} else {
		binary.LittleEndian.PutUint32(frame[12:16], 0x00100001)
	}

	// Inner header
	frame[16] = ChannelAudio                                         // 0x03
	frame[17] = FrameTypeEndSingle                                   // 0x01
	binary.LittleEndian.PutUint16(frame[18:20], uint16(prevFrameNo)) // prev_frame_no (16-bit)

	binary.LittleEndian.PutUint16(frame[20:22], 0x0001) // pkt_total = 1
	binary.LittleEndian.PutUint16(frame[22:24], 0x0010) // flags

	binary.LittleEndian.PutUint32(frame[24:28], uint32(totalPayload)) // payload size
	binary.LittleEndian.PutUint32(frame[28:32], prevFrameNo)          // prev_frame_no again (32-bit)
	binary.LittleEndian.PutUint32(frame[32:36], c.audioTxFrameNo)     // frame_no

	// Audio payload
	copy(frame[headerSize:], payload)

	// FrameInfo (16 bytes) at end of payload
	samplesPerFrame := GetSamplesPerFrame(codec)
	frameDurationMs := samplesPerFrame * 1000 / sampleRate

	fi := frame[headerSize+len(payload):]
	binary.LittleEndian.PutUint16(fi[0:2], codec)            // codec_id
	fi[2] = BuildAudioFlags(sampleRate, true, channels == 2) // flags
	fi[3] = 0                                                // cam_index
	fi[4] = 1                                                // onlineNum = 1
	fi[5] = 0                                                // tags
	// fi[6:12] = reserved (already 0)
	binary.LittleEndian.PutUint32(fi[12:16], (c.audioTxFrameNo-1)*frameDurationMs)

	if c.verbose {
		fmt.Printf("[AUDIO TX] FrameInfo: codec=0x%04x flags=0x%02x online=%d ts=%d\n",
			codec, fi[2], fi[4], binary.LittleEndian.Uint32(fi[12:16]))
	}

	return frame
}

func (c *Conn) buildDisco(stage byte) []byte {
	const bodySize = 72
	const frameSize = 16 + bodySize

	frame := make([]byte, frameSize)

	// IOTC Frame Header [0-15]
	frame[0] = 0x04                                         // [0] Marker1
	frame[1] = 0x02                                         // [1] Marker2
	frame[2] = 0x1a                                         // [2] Marker3
	frame[3] = 0x02                                         // [3] Mode = Disco
	binary.LittleEndian.PutUint16(frame[4:6], bodySize)     // [4-5] BodySize
	binary.LittleEndian.PutUint16(frame[8:10], CmdDiscoReq) // [8-9] Command = 0x0601
	binary.LittleEndian.PutUint16(frame[10:12], 0x0021)     // [10-11] Flags

	// Body [16-87]
	body := frame[16:]
	copy(body[0:20], c.uid) // [0-19] UID (20 bytes)

	body[36] = 0x01 // [36] Unknown1
	body[37] = 0x01 // [37] Unknown2
	body[38] = 0x02 // [38] Unknown3
	body[39] = 0x04 // [39] Unknown4

	copy(body[40:48], c.randomID) // [40-47] RandomID
	body[48] = stage              // [48] Stage (1=broadcast, 2=direct)

	if stage == 1 && len(c.authKey) > 0 {
		copy(body[58:], c.authKey) // [58-65] AuthKey
	}

	return frame
}

func (c *Conn) buildSession() []byte {
	const bodySize = 36
	const frameSize = 16 + bodySize

	frame := make([]byte, frameSize)

	// IOTC Frame Header [0-15]
	frame[0] = 0x04                                           // [0] Marker1
	frame[1] = 0x02                                           // [1] Marker2
	frame[2] = 0x1a                                           // [2] Marker3
	frame[3] = 0x02                                           // [3] Mode
	binary.LittleEndian.PutUint16(frame[4:6], bodySize)       // [4-5] BodySize
	binary.LittleEndian.PutUint16(frame[8:10], CmdSessionReq) // [8-9] Command = 0x0402
	binary.LittleEndian.PutUint16(frame[10:12], 0x0033)       // [10-11] Flags

	// Body [16-51]
	body := frame[16:]
	copy(body[0:20], c.uid)       // [0-19] UID (20 bytes)
	copy(body[20:28], c.randomID) // [20-27] RandomID

	ts := uint32(time.Now().Unix())
	binary.LittleEndian.PutUint32(body[32:36], ts) // [32-35] Timestamp

	return frame
}

func (c *Conn) buildDTLSConfig(isServer bool) *dtls.Config {
	config := &dtls.Config{
		PSK: func(hint []byte) ([]byte, error) {
			if c.verbose {
				fmt.Printf("[DTLS] PSK callback, hint: %s\n", string(hint))
			}
			return c.psk, nil
		},
		PSKIdentityHint:         []byte(PSKIdentity),
		InsecureSkipVerify:      true,
		InsecureSkipVerifyHello: true,
		MTU:                     1200,
		FlightInterval:          300 * time.Millisecond,
		ExtendedMasterSecret:    dtls.DisableExtendedMasterSecret,
	}

	// Use custom cipher suites for client, standard for server
	if isServer {
		config.CipherSuites = []dtls.CipherSuiteID{dtls.TLS_PSK_WITH_AES_128_CBC_SHA256}
	} else {
		config.CustomCipherSuites = CustomCipherSuites
	}

	return config
}

func (c *Conn) buildDataTXChannel(payload []byte, channel byte) []byte {
	const subHeaderSize = 12
	bodySize := subHeaderSize + len(payload)
	frameSize := 16 + bodySize
	frame := make([]byte, frameSize)

	// IOTC Frame Header [0-15]
	frame[0] = 0x04                                             // [0] Marker1
	frame[1] = 0x02                                             // [1] Marker2
	frame[2] = 0x1a                                             // [2] Marker3
	frame[3] = 0x0b                                             // [3] Mode = Data
	binary.LittleEndian.PutUint16(frame[4:6], uint16(bodySize)) // [4-5] BodySize
	binary.LittleEndian.PutUint16(frame[6:8], c.iotcTxSeq)      // [6-7] Sequence
	c.iotcTxSeq++
	binary.LittleEndian.PutUint16(frame[8:10], CmdDataTX) // [8-9] Command = 0x0407
	binary.LittleEndian.PutUint16(frame[10:12], 0x0021)   // [10-11] Flags
	copy(frame[12:14], c.randomID[:2])                    // [12-13] RandomID[0:2]
	frame[14] = channel                                   // [14] Channel (0=Main, 1=Back)
	frame[15] = 0x01                                      // [15] Marker

	// Sub-Header [16-27]
	binary.LittleEndian.PutUint32(frame[16:20], 0x0000000c) // [16-19] Const
	copy(frame[20:28], c.randomID[:8])                      // [20-27] RandomID

	// Payload [28+]
	copy(frame[28:], payload)

	return frame
}

func (c *Conn) buildACK() []byte {
	if c.ackFlags == 0 {
		c.ackFlags = 0x0001
	} else if c.ackFlags < 0x0007 {
		c.ackFlags++
	}

	ack := make([]byte, 24)
	binary.LittleEndian.PutUint16(ack[0:2], MagicACK)        // [0-1] Magic = 0x0009
	binary.LittleEndian.PutUint16(ack[2:4], ProtocolVersion) // [2-3] Version = 0x000C
	binary.LittleEndian.PutUint32(ack[4:8], c.avTxSeq)       // [4-7] TxSeq
	c.avTxSeq++
	binary.LittleEndian.PutUint32(ack[8:12], 0xffffffff)              // [8-11] RxSeq (not used)
	binary.LittleEndian.PutUint16(ack[12:14], c.ackFlags)             // [12-13] AckFlags
	binary.LittleEndian.PutUint32(ack[16:20], uint32(c.ackFlags)<<16) // [16-19] AckCounter

	return ack
}

func (c *Conn) buildKeepaliveResponse(incomingPayload []byte) []byte {
	frame := make([]byte, 24)

	// IOTC Frame Header [0-15]
	frame[0] = 0x04                                             // [0] Marker1
	frame[1] = 0x02                                             // [1] Marker2
	frame[2] = 0x1a                                             // [2] Marker3
	frame[3] = 0x0a                                             // [3] Mode
	binary.LittleEndian.PutUint16(frame[4:6], 8)                // [4-5] BodySize = 8
	binary.LittleEndian.PutUint16(frame[8:10], CmdKeepaliveReq) // [8-9] Command = 0x0427
	binary.LittleEndian.PutUint16(frame[10:12], 0x0021)         // [10-11] Flags

	// Body [16-23]: Echo back incoming payload
	if len(incomingPayload) >= 8 {
		copy(frame[16:24], incomingPayload[:8]) // [16-23] EchoPayload
	}

	return frame
}

func (c *Conn) buildAVLoginPacket(magic uint16, size int, flags uint16, randomID []byte) []byte {
	pkt := make([]byte, size)

	// Header
	binary.LittleEndian.PutUint16(pkt[0:2], magic)
	binary.LittleEndian.PutUint16(pkt[2:4], ProtocolVersion)
	// bytes 4-15: reserved (zeros)

	// Payload info at offset 16
	payloadSize := uint16(size - 24) // total - header(16) - random(4) - padding(4)
	binary.LittleEndian.PutUint16(pkt[16:18], payloadSize)
	binary.LittleEndian.PutUint16(pkt[18:20], flags)
	copy(pkt[20:24], randomID[:4])

	// Credentials (each field is 256 bytes)
	copy(pkt[24:], DefaultUser) // username at offset 24 (payload byte 0)
	copy(pkt[280:], c.enr)      // password (ENR) at offset 280 (payload byte 256)

	// Config section (AVClientStartInConfig) starts at offset 536 (= 24 + 256 + 256)
	// Layout: resend(4) + security_mode(4) + auth_type(4) + sync_recv_data(4) + ...
	binary.LittleEndian.PutUint32(pkt[536:540], 0)                   // resend=0
	binary.LittleEndian.PutUint32(pkt[540:544], 2)                   // security_mode=2 (AV_SECURITY_AUTO)
	binary.LittleEndian.PutUint32(pkt[544:548], 0)                   // auth_type=0 (AV_AUTH_PASSWORD)
	binary.LittleEndian.PutUint32(pkt[548:552], 0)                   // sync_recv_data=0
	binary.LittleEndian.PutUint32(pkt[552:556], DefaultCapabilities) // capabilities
	binary.LittleEndian.PutUint16(pkt[556:558], 0)                   // request_video_on_connect=0
	binary.LittleEndian.PutUint16(pkt[558:560], 0)                   // request_audio_on_connect=0

	return pkt
}

func (c *Conn) buildAVLoginResponse(checksum uint32) []byte {
	resp := make([]byte, 60)

	// Header
	binary.LittleEndian.PutUint16(resp[0:2], 0x2100) // Magic
	binary.LittleEndian.PutUint16(resp[2:4], 0x000c) // Version
	resp[4] = 0x10                                   // Response type (success)

	// Payload info
	binary.LittleEndian.PutUint32(resp[16:20], 0x24)     // Payload size = 36
	binary.LittleEndian.PutUint32(resp[20:24], checksum) // Echo checksum from request!

	// Payload (36 bytes starting at offset 24)
	resp[29] = 0x01 // EnableFlag
	resp[31] = 0x01 // TwoWayStreaming

	binary.LittleEndian.PutUint32(resp[36:40], 0x04)       // BufferConfig
	binary.LittleEndian.PutUint32(resp[40:44], 0x001f07fb) // Capabilities

	binary.LittleEndian.PutUint16(resp[54:56], 0x0003) // ChannelInfo1
	binary.LittleEndian.PutUint16(resp[56:58], 0x0002) // ChannelInfo2

	return resp
}

func (c *Conn) buildIOCtrlFrame(payload []byte) []byte {
	const headerSize = 40
	frame := make([]byte, headerSize+len(payload))

	// Magic (same as protocol version for IOCtrl frames)
	binary.LittleEndian.PutUint16(frame[0:2], ProtocolVersion)

	// Version
	binary.LittleEndian.PutUint16(frame[2:4], ProtocolVersion)

	// AVSeq (4-7)
	seq := c.avTxSeq
	c.avTxSeq++
	binary.LittleEndian.PutUint32(frame[4:8], seq)

	// Bytes 8-15: reserved

	// Channel: MagicIOCtrl (0x7000) for IOCtrl frames
	binary.LittleEndian.PutUint16(frame[16:18], MagicIOCtrl)

	// SubChannel (18-19): increments with each IOCtrl command sent
	binary.LittleEndian.PutUint16(frame[18:20], c.ioctrlSeq)

	// IOCTLSeq (20-23): always 1
	binary.LittleEndian.PutUint32(frame[20:24], 1)

	// PayloadSize (24-27): payload + 4 bytes padding
	binary.LittleEndian.PutUint32(frame[24:28], uint32(len(payload)+4))

	// Flag (28-31): matches subChannel in SDK
	binary.LittleEndian.PutUint32(frame[28:32], uint32(c.ioctrlSeq))

	// Bytes 32-36: reserved
	// Byte 37: 0x01
	frame[37] = 0x01

	// Bytes 38-39: reserved

	// Payload at offset 40
	copy(frame[headerSize:], payload)

	c.ioctrlSeq++

	return frame
}

func (c *Conn) logAVPacket(channel, frameType byte, payload []byte, fi *FrameInfo) {
	fmt.Printf("[Conn] AV: ch=0x%02x type=0x%02x len=%d", channel, frameType, len(payload))
	if fi != nil {
		fmt.Printf(" fi={codec=0x%04x flags=0x%02x ts=%d}", fi.CodecID, fi.Flags, fi.Timestamp)
	}
	fmt.Printf("\n")
}

func (c *Conn) logAudioTX(frame []byte, codec uint16, payloadLen int, timestampUS uint32, sampleRate uint32, channels uint8) {
	chStr := "mono"
	if channels == 2 {
		chStr = "stereo"
	}

	// Determine header size based on frame type
	headerSize := 28
	frameType := "P-Start"
	if len(frame) >= 2 && frame[1] == FrameTypeStartAlt {
		headerSize = 36
		frameType = "Single"
	}

	fmt.Printf("[AUDIO TX] %s codec=0x%04x (%s) payload=%d ts=%d rate=%d %s total=%d\n",
		frameType, codec, AudioCodecName(codec), payloadLen, timestampUS, sampleRate, chStr, len(frame))

	// Dump frame header for comparison with SDK
	if len(frame) >= headerSize {
		fmt.Printf("  HEADER[0..%d]: ", headerSize-1)
		for i := 0; i < headerSize; i++ {
			fmt.Printf("%02x ", frame[i])
		}
		fmt.Printf("\n")
	}

	// First few payload bytes (for comparison with SDK)
	if payloadLen > 0 && len(frame) > headerSize {
		maxShow := min(16, payloadLen)
		fmt.Printf("  PAYLOAD[%d..%d]: ", headerSize, headerSize+maxShow-1)
		for i := 0; i < maxShow; i++ {
			fmt.Printf("%02x ", frame[headerSize+i])
		}
		if payloadLen > maxShow {
			fmt.Printf("...")
		}
		fmt.Printf("\n")
	}
}

func genRandomID() []byte {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return b
}

func getBroadcastAddrs(port int, verbose bool) []*net.UDPAddr {
	var addrs []*net.UDPAddr

	ifaces, err := net.Interfaces()
	if err != nil {
		if verbose {
			fmt.Printf("[IOTC] Failed to get interfaces: %v\n", err)
		}
		// Fallback to limited broadcast
		return []*net.UDPAddr{{IP: net.IPv4(255, 255, 255, 255), Port: port}}
	}

	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		ifAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range ifAddrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// Only IPv4
			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}

			// Calculate broadcast address: IP | ~mask
			mask := ipNet.Mask
			if len(mask) != 4 {
				continue
			}

			broadcast := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				broadcast[i] = ip4[i] | ^mask[i]
			}

			bcastAddr := &net.UDPAddr{IP: broadcast, Port: port}
			addrs = append(addrs, bcastAddr)

			if verbose {
				fmt.Printf("[IOTC] Found broadcast address: %s (iface: %s)\n", bcastAddr, iface.Name)
			}
		}
	}

	if len(addrs) == 0 {
		// Fallback to limited broadcast
		if verbose {
			fmt.Printf("[IOTC] No broadcast addresses found, using 255.255.255.255\n")
		}
		return []*net.UDPAddr{{IP: net.IPv4(255, 255, 255, 255), Port: port}}
	}

	return addrs
}
