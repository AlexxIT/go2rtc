// Package reolink implements two-way audio for Reolink cameras over their own Baichuan protocol
// (TCP 9000). Many Reolink models do not offer an ONVIF backchannel on their RTSP server, while the
// Reolink app can talk to them through Baichuan.
//
// The transport (header layout, XOR key, AES-CFB, nonce login) follows reolink_aio
// (https://github.com/starkillerOG/reolink_aio, MIT).
package reolink

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
)

const (
	cmdLogin       = 1
	cmdLogout      = 2
	cmdTalkAbility = 10
	cmdTalkReset   = 11
	cmdLinkType    = 93 // reolink_aio uses it as keepalive
	cmdTalkConfig  = 201
	cmdTalk        = 202
)

// hostChannel is the channel byte of messages for the device itself, channel N uses N+1
const hostChannel = 250

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" ?>` + "\n"

var (
	magic  = []byte{0xf0, 0xde, 0xbc, 0x0a}
	xmlKey = [8]byte{0x1f, 0x2d, 0x3c, 0x4b, 0x5a, 0x69, 0x78, 0xff}
	aesIV  = []byte("0123456789abcdef")
)

// message classes, bytes 18-19 of the header
var (
	classLegacy = [2]byte{0x14, 0x65} // 20 byte header, only used to request the nonce
	classModern = [2]byte{0x14, 0x64} // 24 byte header with payload offset
	classReply  = [2]byte{0x14, 0x66} // 20 byte reply header
	classZero   = [2]byte{0x00, 0x00} // 24 byte reply header
)

type message struct {
	cmd    uint32
	id     uint32 // channel byte + 24 bit sequence number
	status uint16 // only in 24 byte headers
	ext    []byte
	body   []byte
}

// conn is one Baichuan session. A background goroutine reads all messages and hands replies to
// the request that waits for them; everything else (events, replies to Talk) is dropped.
type conn struct {
	net.Conn
	key []byte // AES key, known after the nonce exchange

	seq atomic.Uint32
	wmu sync.Mutex

	// readTimeout detects a dead connection; the camera answers the keepalive well within it
	readTimeout time.Duration

	mu      sync.Mutex
	waiters map[uint32]chan *message
	err     error
	done    chan struct{}
}

func dial(address string, readTimeout time.Duration) (*conn, error) {
	nc, err := net.DialTimeout("tcp", address, core.ConnDialTimeout)
	if err != nil {
		return nil, err
	}
	c := &conn{Conn: nc, readTimeout: readTimeout, waiters: map[uint32]chan *message{}, done: make(chan struct{})}
	go c.readLoop()
	return c, nil
}

func (c *conn) nextID(channel byte) uint32 {
	return uint32(channel) | (c.seq.Add(1)&0xffffff)<<8
}

func (c *conn) write(cmd, id uint32, class [2]byte, ext, body []byte) error {
	b := make([]byte, 0, 24+len(ext)+len(body))
	b = append(b, magic...)
	b = binary.LittleEndian.AppendUint32(b, cmd)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(ext)+len(body)))
	b = binary.LittleEndian.AppendUint32(b, id)
	if class == classLegacy {
		b = append(b, 0x12, 0xdc, class[0], class[1])
	} else {
		b = append(b, 0, 0, class[0], class[1])
		b = binary.LittleEndian.AppendUint32(b, uint32(len(ext)))
	}
	b = append(b, ext...)
	b = append(b, body...)

	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.SetWriteDeadline(time.Now().Add(core.ConnDeadline))
	_, err := c.Conn.Write(b)
	return err
}

// roundTrip writes a message and waits for the reply with the same id
func (c *conn) roundTrip(cmd, id uint32, class [2]byte, ext, body []byte) (*message, error) {
	wait := make(chan *message, 1)

	c.mu.Lock()
	if c.waiters == nil {
		c.mu.Unlock()
		return nil, c.closedErr()
	}
	c.waiters[id] = wait
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		if c.waiters != nil {
			delete(c.waiters, id)
		}
		c.mu.Unlock()
	}()

	if err := c.write(cmd, id, class, ext, body); err != nil {
		return nil, err
	}

	select {
	case msg, ok := <-wait:
		if !ok {
			return nil, c.closedErr()
		}
		if msg.cmd != cmd {
			return nil, fmt.Errorf("reolink: reply to cmd %d has cmd %d", cmd, msg.cmd)
		}
		return msg, nil
	case <-time.After(core.ConnDeadline):
		return nil, fmt.Errorf("reolink: no reply to cmd %d", cmd)
	}
}

// request sends an AES encrypted message to a channel byte and checks the reply status
func (c *conn) request(cmd uint32, channel byte, ext, body string) (*message, error) {
	msg, err := c.roundTrip(cmd, c.nextID(channel), classModern, c.encrypt([]byte(ext)), c.encrypt([]byte(body)))
	if err != nil {
		return nil, err
	}
	if msg.status != 200 {
		return msg, &statusError{cmd: cmd, status: msg.status}
	}
	return msg, nil
}

type statusError struct {
	cmd    uint32
	status uint16
}

func (e *statusError) Error() string {
	return fmt.Sprintf("reolink: cmd %d status %d", e.cmd, e.status)
}

func (c *conn) closedErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return net.ErrClosed
}

func (c *conn) readLoop() {
	defer close(c.done)

	rd := bufio.NewReader(c.Conn)
	for {
		if c.readTimeout > 0 {
			_ = c.SetReadDeadline(time.Now().Add(c.readTimeout))
		}

		msg, err := readMessage(rd)
		if err != nil {
			// close the socket too, otherwise a reconnect leaves this session open on the camera
			_ = c.Conn.Close()

			c.mu.Lock()
			c.err = err
			for _, wait := range c.waiters {
				close(wait)
			}
			c.waiters = nil
			c.mu.Unlock()
			return
		}

		c.mu.Lock()
		wait := c.waiters[msg.id]
		delete(c.waiters, msg.id)
		c.mu.Unlock()

		if wait != nil {
			wait <- msg
		}
	}
}

func readMessage(rd io.Reader) (*message, error) {
	header := make([]byte, 20)
	if _, err := io.ReadFull(rd, header); err != nil {
		return nil, err
	}
	if string(header[:4]) != string(magic) {
		return nil, fmt.Errorf("reolink: wrong magic %x", header[:4])
	}

	msg := &message{
		cmd: binary.LittleEndian.Uint32(header[4:]),
		id:  binary.LittleEndian.Uint32(header[12:]),
	}
	size := binary.LittleEndian.Uint32(header[8:])

	var payloadOffset uint32
	switch class := [2]byte(header[18:20]); class {
	case classReply:
		msg.status = 200 // 20 byte replies carry no status, reolink_aio treats them as success
	case classModern, classZero:
		extra := make([]byte, 4)
		if _, err := io.ReadFull(rd, extra); err != nil {
			return nil, err
		}
		payloadOffset = binary.LittleEndian.Uint32(extra)
		msg.status = binary.LittleEndian.Uint16(header[16:])
	default:
		return nil, fmt.Errorf("reolink: unknown message class %x", class)
	}

	if size > 1<<22 {
		return nil, fmt.Errorf("reolink: message too big: %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(rd, payload); err != nil {
		return nil, err
	}
	if payloadOffset > 0 && payloadOffset < size {
		msg.ext, msg.body = payload[:payloadOffset], payload[payloadOffset:]
	} else {
		msg.body = payload
	}
	return msg, nil
}

func (c *conn) login(user, pass string) error {
	// an empty message of the legacy class makes the camera answer with a nonce
	msg, err := c.roundTrip(cmdLogin, c.nextID(hostChannel), classLegacy, nil, nil)
	if err != nil {
		return err
	}
	nonce := core.Between(string(xorBC(msg.body, byte(msg.id))), "<nonce>", "<")
	if nonce == "" {
		return errors.New("reolink: no nonce in reply")
	}
	c.key = []byte(md5Modern(nonce + "-" + pass)[:16])

	body := xmlHeader + "<body>\n" +
		"<LoginUser version=\"1.1\">\n" +
		"<userName>" + md5Modern(user+nonce) + "</userName>\n" +
		"<password>" + md5Modern(pass+nonce) + "</password>\n" +
		"<userVer>1</userVer>\n" +
		"</LoginUser>\n" +
		"<LoginNet version=\"1.1\">\n<type>LAN</type>\n<udpPort>0</udpPort>\n</LoginNet>\n" +
		"</body>\n"

	msg, err = c.roundTrip(cmdLogin, c.nextID(hostChannel), classModern, nil, xorBC([]byte(body), hostChannel))
	if err != nil {
		return err
	}
	switch msg.status {
	case 200:
		return nil
	case 401:
		return errors.New("reolink: wrong username or password")
	}
	return fmt.Errorf("reolink: login status %d", msg.status)
}

// closeGracefully lets the last messages reach the camera: closing a socket with unread data sends
// a reset, which can drop what is still in the send buffer (the TalkReset and the logout)
func (c *conn) closeGracefully() {
	if tcp, ok := c.Conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
		select {
		case <-c.done: // the camera closed its side, the read loop drained everything
		case <-time.After(time.Second):
		}
	}
	_ = c.Close()
}

func (c *conn) logout() {
	body := xmlHeader + "<body>\n<LoginUser version=\"1.1\">\n<userVer>1</userVer>\n</LoginUser>\n</body>\n"
	_ = c.write(cmdLogout, c.nextID(hostChannel), classModern, nil, c.encrypt([]byte(body)))
}

// md5Modern is the Baichuan flavour of MD5: upper case hex without the last character
func md5Modern(s string) string {
	sum := md5.Sum([]byte(s))
	return strings.ToUpper(hex.EncodeToString(sum[:])[:31])
}

// xorBC encrypts and decrypts with the static Baichuan key, offset is the message channel byte
func xorBC(b []byte, offset byte) []byte {
	dst := make([]byte, len(b))
	for i, v := range b {
		dst[i] = v ^ xmlKey[(int(offset)+i)%len(xmlKey)] ^ offset
	}
	return dst
}

// encrypt runs AES-128-CFB from the fixed IV, every section of a message is its own stream
func (c *conn) encrypt(b []byte) []byte {
	return c.aesCFB(b, false)
}

// decryptXML returns the XML body of a reply, which some firmwares send with the static key instead of AES
func (c *conn) decryptXML(msg *message) string {
	if b := c.aesCFB(msg.body, true); strings.HasPrefix(string(b), "<?xml") {
		return string(b)
	}
	if b := xorBC(msg.body, byte(msg.id)); strings.HasPrefix(string(b), "<?xml") {
		return string(b)
	}
	return string(msg.body)
}

func (c *conn) aesCFB(b []byte, decrypt bool) []byte {
	if len(b) == 0 {
		return nil
	}
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil
	}
	var stream cipher.Stream
	if decrypt {
		stream = cipher.NewCFBDecrypter(block, aesIV) //nolint:staticcheck // the protocol uses CFB
	} else {
		stream = cipher.NewCFBEncrypter(block, aesIV) //nolint:staticcheck
	}
	dst := make([]byte, len(b))
	stream.XORKeyStream(dst, b)
	return dst
}
