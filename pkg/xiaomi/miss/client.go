package miss

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"golang.org/x/crypto/chacha20"
	"golang.org/x/crypto/nacl/box"
)

func Dial(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	if s := query.Get("vendor"); s != "cs2" {
		return nil, fmt.Errorf("miss: unsupported vendor %s", s)
	}

	clientPrivate := query.Get("client_private")
	devicePublic := query.Get("device_public")

	key, err := calcSharedKey(devicePublic, clientPrivate)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		return nil, err
	}

	client := &Client{
		conn: conn,
		addr: &net.UDPAddr{IP: net.ParseIP(u.Host), Port: 32108},
		buf:  make([]byte, 1500),
		key:  key,
	}

	clientPublic := query.Get("client_public")
	sign := query.Get("sign")

	if err = client.login(clientPublic, sign); err != nil {
		_ = conn.Close()
		return nil, err
	}

	client.chSeq0 = 1
	client.chRaw2 = make(chan []byte, 100)
	go client.worker()

	return client, nil
}

const (
	CodecH264 = 4
	CodecH265 = 5
	CodecPCM  = 1024
	CodecPCMU = 1026
	CodecPCMA = 1027
	CodecOPUS = 1032
)

type Client struct {
	conn *net.UDPConn
	addr *net.UDPAddr
	buf  []byte
	key  []byte // shared key

	chSeq0 uint16
	chSeq3 uint16
	chRaw2 chan []byte
}

func (c *Client) RemoteAddr() *net.UDPAddr {
	return c.addr
}

func (c *Client) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *Client) Close() error {
	return c.conn.Close()
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
	_ = c.conn.SetDeadline(time.Now().Add(core.ConnDialTimeout))

	buf, err := c.writeAndWait([]byte{magic, msgLanSearch, 0, 0}, msgPunchPkt)
	if err != nil {
		return fmt.Errorf("miss: read punch: %w", err)
	}

	_, err = c.writeAndWait(buf, msgP2PRdy)
	if err != nil {
		return fmt.Errorf("miss: read ready: %w", err)
	}

	_, _ = c.conn.WriteToUDP([]byte{magic, msgAlive, 0, 0}, c.addr)

	s := fmt.Sprintf(`{"public_key":"%s","sign":"%s","uuid":"","support_encrypt":0}`, clientPublic, sign)
	buf, err = c.writeAndWait(marshalCmd(0, 0, cmdAuthReq, []byte(s)), msgDrw)
	if err != nil {
		return fmt.Errorf("miss: read auth: %w", err)
	}

	if !strings.Contains(string(buf[16:]), `"result":"success"`) {
		return fmt.Errorf("miss: read auth: %s", buf[16:])
	}

	_, _ = c.conn.WriteToUDP([]byte{magic, msgDrwAck, 0, 6, magicDrw, 0, 0, 1, 0, 0}, c.addr)

	_ = c.conn.SetDeadline(time.Time{})

	return nil
}

func (c *Client) writeAndWait(b []byte, waitMsg uint8) ([]byte, error) {
	if _, err := c.conn.WriteToUDP(b, c.addr); err != nil {
		return nil, err
	}

	for {
		n, addr, err := c.conn.ReadFromUDP(c.buf)
		if err != nil {
			return nil, err
		}

		if string(addr.IP) != string(c.addr.IP) {
			continue // skip messages from another IP
		}

		if n >= 16 && c.buf[0] == magic && c.buf[1] == waitMsg {
			if waitMsg == msgPunchPkt {
				c.addr.Port = addr.Port
			}
			return c.buf[:n], nil
		}
	}
}

func (c *Client) VideoStart(channel, quality, audio uint8) error {
	buf := binary.BigEndian.AppendUint32(nil, cmdVideoStart)
	if channel == 0 {
		buf = fmt.Appendf(buf, `{"videoquality":%d,"enableaudio":%d}`, quality, audio)
	} else {
		buf = fmt.Appendf(buf, `{"videoquality":-1,"videoquality2":%d,"enableaudio":%d}`, quality, audio)
	}
	buf, err := encode(c.key, buf)
	if err != nil {
		return err
	}
	buf = marshalCmd(0, c.chSeq0, cmdEncoded, buf)
	c.chSeq0++

	_, err = c.conn.WriteToUDP(buf, c.addr)
	return err
}

func (c *Client) VideoStartDual(qualityMain, qualitySub, audio uint8) error {
	buf := binary.BigEndian.AppendUint32(nil, cmdVideoStart)
	buf = fmt.Appendf(buf, `{"videoquality":%d,"videoquality2":%d,"enableaudio":%d}`, qualityMain, qualitySub, audio)
	buf, err := encode(c.key, buf)
	if err != nil {
		return err
	}
	buf = marshalCmd(0, c.chSeq0, cmdEncoded, buf)
	c.chSeq0++

	_, err = c.conn.WriteToUDP(buf, c.addr)
	return err
}

func (c *Client) SpeakerStart() error {
	buf := binary.BigEndian.AppendUint32(nil, cmdSpeakerStartReq)
	buf, err := encode(c.key, buf)
	if err != nil {
		return err
	}
	buf = marshalCmd(0, c.chSeq0, cmdEncoded, buf)
	c.chSeq0++

	_, err = c.conn.WriteToUDP(buf, c.addr)
	return err
}

func (c *Client) ReadPacket() (*Packet, error) {
	b, ok := <-c.chRaw2
	if !ok {
		return nil, fmt.Errorf("miss: read raw: i/o timeout")
	}
	return unmarshalPacket(c.key, b)
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

	frameType := binary.LittleEndian.Uint32(b[24:28])
	channel := b[28]
	channelOK := channel <= 1 && frameType <= 3

	return &Packet{
		CodecID:   binary.LittleEndian.Uint32(b[4:]),
		Sequence:  binary.LittleEndian.Uint32(b[8:]),
		Flags:     binary.LittleEndian.Uint32(b[12:]),
		Timestamp: binary.LittleEndian.Uint64(b[16:]),
		FrameType: frameType,
		Channel:   channel,
		ChannelOK: channelOK,
		Payload:   payload,
	}, nil
}

func (c *Client) WriteAudio(codecID uint32, payload []byte) error {
	payload, err := encode(c.key, payload)
	if err != nil {
		return err
	}

	n := uint32(len(payload))

	const hdrOffset = 12
	const hdrSize = 32

	buf := make([]byte, n+hdrOffset+hdrSize)
	buf[0] = magic
	buf[1] = msgDrw
	binary.BigEndian.PutUint16(buf[2:], uint16(n+8+hdrSize))

	buf[4] = magicDrw
	buf[5] = 3 // channel
	binary.BigEndian.PutUint16(buf[6:], c.chSeq3)

	binary.BigEndian.PutUint32(buf[8:], n+hdrSize)

	binary.LittleEndian.PutUint32(buf[hdrOffset:], n)
	binary.LittleEndian.PutUint32(buf[hdrOffset+4:], codecID)
	binary.LittleEndian.PutUint64(buf[hdrOffset+16:], uint64(time.Now().UnixMilli()))
	copy(buf[hdrOffset+hdrSize:], payload)

	c.chSeq3++

	_, err = c.conn.WriteToUDP(buf, c.addr)
	return err
}

func (c *Client) worker() {
	defer close(c.chRaw2)

	chAck := []uint16{1, 0, 0, 0}

	var ch2WaitSize int
	var ch2WaitData []byte

	for {
		n, addr, err := c.conn.ReadFromUDP(c.buf)
		if err != nil {
			return
		}

		//log.Printf("<- %.20x...", c.buf[:n])

		if string(addr.IP) != string(c.addr.IP) || n < 8 || c.buf[0] != magic {
			//log.Printf("unknown msg: %x", c.buf[:n])
			continue // skip messages from another IP
		}

		switch c.buf[1] {
		case msgDrw:
			ch := c.buf[5]
			seqHI := c.buf[6]
			seqLO := c.buf[7]

			if chAck[ch] != uint16(seqHI)<<8|uint16(seqLO) {
				continue
			}
			chAck[ch]++

			//log.Printf("%.40x", c.buf)

			ack := []byte{magic, msgDrwAck, 0, 6, magicDrw, ch, 0, 1, seqHI, seqLO}
			if _, err = c.conn.WriteToUDP(ack, c.addr); err != nil {
				return
			}

			switch ch {
			case 0:
				//log.Printf("data ch0 %x", c.buf[:n])
				//size := binary.BigEndian.Uint32(c.buf[8:])
				//if binary.BigEndian.Uint32(c.buf[12:]) == cmdEncoded {
				//	raw, _ := decode(c.key, c.buf[16:12+size])
				//	log.Printf("cmd enc %x", raw)
				//} else {
				//	log.Printf("cmd raw %x", c.buf[12:12+size])
				//}

			case 2:
				ch2WaitData = append(ch2WaitData, c.buf[8:n]...)

				for len(ch2WaitData) > 4 {
					if ch2WaitSize == 0 {
						ch2WaitSize = int(binary.BigEndian.Uint32(ch2WaitData))
						ch2WaitData = ch2WaitData[4:]
					}
					if ch2WaitSize <= len(ch2WaitData) {
						c.chRaw2 <- ch2WaitData[:ch2WaitSize]
						ch2WaitData = ch2WaitData[ch2WaitSize:]
						ch2WaitSize = 0
					} else {
						break
					}
				}

			default:
				log.Printf("!!! unknown chanel: %x", c.buf[:n])
			}

		case msgDrwAck: // skip it

		default:
			log.Printf("!!! unknown msg type: %x", c.buf[:n])
		}
	}
}

func marshalCmd(channel byte, seq uint16, cmd uint32, payload []byte) []byte {
	size := len(payload)
	buf := make([]byte, 4+4+4+4+size)

	// 1. message header (4 bytes)
	buf[0] = magic
	buf[1] = msgDrw
	binary.BigEndian.PutUint16(buf[2:], uint16(4+4+4+size))

	// 2. drw? header (4 bytes)
	buf[4] = magicDrw
	buf[5] = channel
	binary.BigEndian.PutUint16(buf[6:], seq)

	// 3. payload size (4 bytes)
	binary.BigEndian.PutUint32(buf[8:], uint32(4+size))

	// 4. payload command (4 bytes)
	binary.BigEndian.PutUint32(buf[12:], cmd)

	// 5. payload
	copy(buf[16:], payload)

	return buf
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
	FrameType uint32
	Channel   uint8
	ChannelOK bool
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
