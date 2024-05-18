package dvrip

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"
)

const (
	Login          = 1000
	OPMonitorClaim = 1413
	OPMonitorStart = 1410
	OPTalkClaim    = 1434
	OPTalkStart    = 1430
	OPTalkData     = 1432
)

type Client struct {
	conn    net.Conn
	session uint32
	seq     uint32
	stream  string

	rd  io.Reader
	buf []byte
}

func (c *Client) Dial(rawURL string) (err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}

	if u.Port() == "" {
		// add default TCP port
		u.Host += ":34567"
	}

	c.conn, err = net.DialTimeout("tcp", u.Host, time.Second*3)
	if err != nil {
		return
	}

	if query := u.Query(); query.Get("backchannel") != "1" {
		channel := query.Get("channel")
		if channel == "" {
			channel = "0"
		}

		subtype := query.Get("subtype")
		switch subtype {
		case "", "0":
			subtype = "Main"
		case "1":
			subtype = "Extra1"
		}

		c.stream = fmt.Sprintf(
			`{"Channel":%s,"CombinMode":"NONE","StreamType":"%s","TransMode":"TCP"}`,
			channel, subtype,
		)
	}

	c.rd = bufio.NewReader(c.conn)

	if u.User != nil {
		pass, _ := u.User.Password()
		return c.Login(u.User.Username(), pass)
	} else {
		return c.Login("admin", "admin")
	}
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Login(user, pass string) (err error) {
	data := fmt.Sprintf(
		`{"EncryptType":"MD5","LoginType":"DVRIP-Web","PassWord":"%s","UserName":"%s"}`+"\x0A\x00",
		SofiaHash(pass), user,
	)

	if _, err = c.WriteCmd(Login, []byte(data)); err != nil {
		return
	}

	_, err = c.ReadJSON()
	return
}

func (c *Client) Play() error {
	format := `{"Name":"OPMonitor","SessionID":"0x%08X","OPMonitor":{"Action":"%s","Parameter":%s}}` + "\x0A\x00"

	data := fmt.Sprintf(format, c.session, "Claim", c.stream)
	if _, err := c.WriteCmd(OPMonitorClaim, []byte(data)); err != nil {
		return err
	}
	if _, err := c.ReadJSON(); err != nil {
		return err
	}

	data = fmt.Sprintf(format, c.session, "Start", c.stream)
	_, err := c.WriteCmd(OPMonitorStart, []byte(data))
	return err
}

func (c *Client) Talk() error {
	format := `{"Name":"OPTalk","SessionID":"0x%08X","OPTalk":{"Action":"%s","AudioFormat":{"EncodeType":"G711_ALAW"}}}` + "\x0A\x00"

	data := fmt.Sprintf(format, c.session, "Claim")
	if _, err := c.WriteCmd(OPTalkClaim, []byte(data)); err != nil {
		return err
	}
	if _, err := c.ReadJSON(); err != nil {
		return err
	}

	data = fmt.Sprintf(format, c.session, "Start")
	_, err := c.WriteCmd(OPTalkStart, []byte(data))
	return err
}

func (c *Client) WriteCmd(cmd uint16, payload []byte) (n int, err error) {
	b := make([]byte, 20, 128)
	b[0] = 255
	binary.LittleEndian.PutUint32(b[4:], c.session)
	binary.LittleEndian.PutUint32(b[8:], c.seq)
	binary.LittleEndian.PutUint16(b[14:], cmd)
	binary.LittleEndian.PutUint32(b[16:], uint32(len(payload)))
	b = append(b, payload...)

	c.seq++

	if err = c.conn.SetWriteDeadline(time.Now().Add(time.Second * 5)); err != nil {
		return 0, err
	}

	return c.conn.Write(b)
}

func (c *Client) ReadChunk() (b []byte, err error) {
	if err = c.conn.SetReadDeadline(time.Now().Add(time.Second * 5)); err != nil {
		return
	}

	b = make([]byte, 20)
	if _, err = io.ReadFull(c.rd, b); err != nil {
		return
	}

	if b[0] != 255 {
		return nil, errors.New("read error")
	}

	c.session = binary.LittleEndian.Uint32(b[4:])
	size := binary.LittleEndian.Uint32(b[16:])

	b = make([]byte, size)
	if _, err = io.ReadFull(c.rd, b); err != nil {
		return
	}

	return
}

func (c *Client) ReadPacket() (pType byte, payload []byte, err error) {
	var b []byte

	// many cameras may split packet to multiple chunks
	// some rare cameras may put multiple packets to single chunk
	for len(c.buf) < 16 {
		if b, err = c.ReadChunk(); err != nil {
			return 0, nil, err
		}
		c.buf = append(c.buf, b...)
	}

	if !bytes.HasPrefix(c.buf, []byte{0, 0, 1}) {
		return 0, nil, fmt.Errorf("dvrip: wrong packet: %0.16x", c.buf)
	}

	var size int

	switch pType = c.buf[3]; pType {
	case 0xFC, 0xFE:
		size = int(binary.LittleEndian.Uint32(c.buf[12:])) + 16
	case 0xFD: // PFrame
		size = int(binary.LittleEndian.Uint32(c.buf[4:])) + 8
	case 0xFA, 0xF9:
		size = int(binary.LittleEndian.Uint16(c.buf[6:])) + 8
	default:
		return 0, nil, fmt.Errorf("dvrip: unknown packet type: %X", pType)
	}

	for len(c.buf) < size {
		if b, err = c.ReadChunk(); err != nil {
			return 0, nil, err
		}
		c.buf = append(c.buf, b...)
	}

	payload = c.buf[:size]
	c.buf = c.buf[size:]

	return
}

type Response map[string]any

func (c *Client) ReadJSON() (res Response, err error) {
	b, err := c.ReadChunk()
	if err != nil {
		return
	}

	res = Response{}
	if err = json.Unmarshal(b[:len(b)-2], &res); err != nil {
		return
	}

	if v, ok := res["Ret"].(float64); !ok || (v != 100 && v != 515) {
		err = fmt.Errorf("wrong response: %s", b)
	}
	return
}

func SofiaHash(password string) string {
	const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	sofia := make([]byte, 0, 8)
	hash := md5.Sum([]byte(password))
	for i := 0; i < md5.Size; i += 2 {
		j := uint16(hash[i]) + uint16(hash[i+1])
		sofia = append(sofia, chars[j%62])
	}

	return string(sofia)
}
