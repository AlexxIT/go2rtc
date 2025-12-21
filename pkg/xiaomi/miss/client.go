package miss

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi/cs2"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/tutk"
	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/nacl/box"
)

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()

	c := &Client{}

	c.key, err = calcSharedKey(query.Get("device_public"), query.Get("client_private"))
	if err != nil {
		return nil, err
	}

	switch s := query.Get("vendor"); s {
	case "cs2":
		c.conn, err = cs2.Dial(u.Host)
	case "tutk":
		c.conn, err = tutk.Dial(u.Host, query.Get("uid"))
	default:
		return nil, fmt.Errorf("miss: unsupported vendor %s", s)
	}

	if err != nil {
		return nil, err
	}

	err = c.login(query.Get("client_public"), query.Get("sign"))
	if err != nil {
		_ = c.conn.Close()
		return nil, err
	}

	return c, nil
}

const (
	CodecH264 = 4
	CodecH265 = 5
	CodecPCM  = 1024
	CodecPCMU = 1026
	CodecPCMA = 1027
	CodecOPUS = 1032
)

type Conn interface {
	ReadCommand() (cmd uint16, data []byte, err error)
	WriteCommand(cmd uint16, data []byte) error
	ReadPacket() ([]byte, error)
	WritePacket(data []byte) error
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	Close() error
}

type Client struct {
	conn Conn
	key  []byte
}

func (c *Client) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *Client) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) Protocol() string {
	switch c.conn.(type) {
	case *cs2.Conn:
		return "cs2+udp"
	case *tutk.Conn:
		return "tutk+udp"
	}
	return ""
}

const (
	cmdAuthReq           = 0x100
	cmdAuthRes           = 0x101
	cmdVideoStart        = 0x102
	cmdVideoStop         = 0x103
	cmdAudioStart        = 0x104
	cmdAudioStop         = 0x105
	cmdSpeakerStartReq   = 0x106
	cmdSpeakerStartRes   = 0x107
	cmdSpeakerStop       = 0x108
	cmdStreamCtrlReq     = 0x109
	cmdStreamCtrlRes     = 0x10A
	cmdGetAudioFormatReq = 0x10B
	cmdGetAudioFormatRes = 0x10C
	cmdPlaybackReq       = 0x10D
	cmdPlaybackRes       = 0x10E
	cmdDevInfoReq        = 0x110
	cmdDevInfoRes        = 0x111
	cmdMotorReq          = 0x112
	cmdMotorRes          = 0x113
	cmdEncoded           = 0x1001
)

func (c *Client) login(clientPublic, sign string) error {
	s := fmt.Sprintf(`{"public_key":"%s","sign":"%s","uuid":"","support_encrypt":0}`, clientPublic, sign)
	if err := c.conn.WriteCommand(cmdAuthReq, []byte(s)); err != nil {
		return err
	}

	_, data, err := c.conn.ReadCommand()
	if err != nil {
		return err
	}

	if !bytes.Contains(data, []byte(`"result":"success"`)) {
		return fmt.Errorf("miss: auth: %s", data)
	}

	return nil
}

func (c *Client) WriteCommand(data []byte) error {
	data, err := encode(c.key, data)
	if err != nil {
		return err
	}
	return c.conn.WriteCommand(cmdEncoded, data)
}

func (c *Client) VideoStart(channel, quality, audio uint8) error {
	data := binary.BigEndian.AppendUint32(nil, cmdVideoStart)
	if channel == 0 {
		data = fmt.Appendf(data, `{"videoquality":%d,"enableaudio":%d}`, quality, audio)
	} else {
		data = fmt.Appendf(data, `{"videoquality":-1,"videoquality2":%d,"enableaudio":%d}`, quality, audio)
	}
	return c.WriteCommand(data)
}

func (c *Client) AudioStart() error {
	data := binary.BigEndian.AppendUint32(nil, cmdAudioStart)
	return c.WriteCommand(data)
}

func (c *Client) SpeakerStart() error {
	data := binary.BigEndian.AppendUint32(nil, cmdSpeakerStartReq)
	return c.WriteCommand(data)
}

func (c *Client) ReadPacket() (*Packet, error) {
	data, err := c.conn.ReadPacket()
	if err != nil {
		return nil, fmt.Errorf("miss: read media: %w", err)
	}
	return unmarshalPacket(c.key, data)
}

func unmarshalPacket(key, b []byte) (*Packet, error) {
	n := uint32(len(b))

	if n < 32 {
		return nil, fmt.Errorf("miss: packet header too small")
	}

	if l := binary.LittleEndian.Uint32(b); l+32 != n {
		return nil, fmt.Errorf("miss: packet payload has wrong length")
	}

	payload, err := decode(key, b[32:])
	if err != nil {
		return nil, err
	}

	return &Packet{
		CodecID:   binary.LittleEndian.Uint32(b[4:]),
		Sequence:  binary.LittleEndian.Uint32(b[8:]),
		Flags:     binary.LittleEndian.Uint32(b[12:]),
		Timestamp: binary.LittleEndian.Uint64(b[16:]),
		Payload:   payload,
	}, nil
}

func (c *Client) WriteAudio(codecID uint32, payload []byte) error {
	payload, err := encode(c.key, payload) // new payload will have new size!
	if err != nil {
		return err
	}

	const hdrSize = 32
	n := uint32(len(payload))

	data := make([]byte, hdrSize+n)
	binary.LittleEndian.PutUint32(data, n)
	binary.LittleEndian.PutUint32(data[4:], codecID)
	binary.LittleEndian.PutUint64(data[16:], uint64(time.Now().UnixMilli())) // not really necessary
	copy(data[hdrSize:], payload)
	return c.conn.WritePacket(data)
}

func calcSharedKey(devicePublic, clientPrivate string) ([]byte, error) {
	var sharedKey, publicKey, privateKey [32]byte
	if _, err := hex.Decode(publicKey[:], []byte(devicePublic)); err != nil {
		return nil, err
	}
	if _, err := hex.Decode(privateKey[:], []byte(clientPrivate)); err != nil {
		return nil, err
	}
	box.Precompute(&sharedKey, &publicKey, &privateKey)
	return sharedKey[:], nil
}

func encode(key, src []byte) ([]byte, error) {
	dst := make([]byte, len(src)+8)

	if _, err := rand.Read(dst[:8]); err != nil {
		return nil, err
	}

	nonce := make([]byte, 12)
	copy(nonce[4:], dst[:8])

	c, err := chacha20.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		return nil, err
	}

	c.XORKeyStream(dst[8:], src)

	return dst, nil
}

func decode(key, src []byte) ([]byte, error) {
	nonce := make([]byte, 12)
	copy(nonce[4:], src[:8])

	c, err := chacha20.NewUnauthenticatedCipher(key, nonce)
	if err != nil {
		return nil, err
	}

	dst := make([]byte, len(src)-8)
	c.XORKeyStream(dst, src[8:])

	return dst, nil
}

type Packet struct {
	//Length    uint32
	CodecID   uint32
	Sequence  uint32
	Flags     uint32
	Timestamp uint64 // msec
	//TimestampS uint32
	//Reserved uint32
	Payload []byte
}

func GenerateKey() ([]byte, []byte, error) {
	public, private, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return public[:], private[:], err
}
