package miss

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss/cs2"
)

const (
	codecH264 = 4
	codecH265 = 5
	codecPCM  = 1024
	codecPCMU = 1026
	codecPCMA = 1027
	codecOPUS = 1032
)

type Conn interface {
	Protocol() string
	Version() string
	ReadCommand() (cmd uint32, data []byte, err error)
	WriteCommand(cmd uint32, data []byte) error
	WriteCommandTo(channel byte, cmd uint32, data []byte) error
	ReadPacket() (hdr, payload []byte, err error)
	WritePacket(hdr, payload []byte) error
	RemoteAddr() net.Addr
	SetDeadline(t time.Time) error
	Close() error
}

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	// 1. Check if we can create shared key.
	query := u.Query()
	key, err := crypto.CalcSharedKey(query.Get("device_public"), query.Get("client_private"))
	if err != nil {
		return nil, err
	}

	model := query.Get("model")

	// 2. Check if this vendor supported.
	var conn Conn
	switch s := query.Get("vendor"); s {
	case "cs2":
		conn, err = cs2.Dial(u.Host, query.Get("transport"))
	case "tutk":
		conn, err = tutk.Dial(u.Host, query.Get("uid"), "Miss", "client")
	default:
		err = fmt.Errorf("miss: unsupported vendor %s", s)
	}

	if err != nil {
		return nil, err
	}

	err = login(conn, query.Get("client_public"), query.Get("sign"))
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Client{Conn: conn, key: key, model: model}, nil
}

type Client struct {
	Conn
	key   []byte
	model string
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

func login(conn Conn, clientPublic, sign string) error {
	uuid := generateUUID()
	s := fmt.Sprintf(`{"public_key":"%s","sign":"%s","uuid":"%s","support_encrypt":15}`, clientPublic, sign, uuid)
	fmt.Printf("[xiaomi-debug] login auth: %s\n", s)
	if err := conn.WriteCommand(cmdAuthReq, []byte(s)); err != nil {
		return err
	}

	_, data, err := conn.ReadCommand()
	if err != nil {
		return err
	}

	if !bytes.Contains(data, []byte(`"result":"success"`)) {
		return fmt.Errorf("miss: auth: %s", data)
	}

	return nil
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	// Set version 4 and variant bits per RFC 4122
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (c *Client) Version() string {
	return fmt.Sprintf("%s (%s)", c.Conn.Version(), c.model)
}

func (c *Client) WriteCommand(data []byte) error {
	data, err := crypto.Encode(data, c.key)
	if err != nil {
		return err
	}
	return c.Conn.WriteCommand(cmdEncoded, data)
}

func (c *Client) WriteCommandTo(channel byte, data []byte) error {
	data, err := crypto.Encode(data, c.key)
	if err != nil {
		return err
	}
	return c.Conn.WriteCommandTo(channel, cmdEncoded, data)
}

const (
	ModelDafang    = "isa.camera.df3"
	ModelLoockV2   = "loock.cateye.v02"
	ModelC200      = "chuangmi.camera.046c04"
	ModelC300      = "chuangmi.camera.72ac1"
	ModelDoorbell2 = "madv.cateye.miowlv2"
	// ModelXiaofang looks like it has the same firmware as the ModelDafang.
	// There is also an older model "isa.camera.isc5" that only works with the legacy protocol.
	ModelXiaofang = "isa.camera.isc5c1"
)

func (c *Client) StartMedia(channel, quality, audio string) error {
	switch c.model {
	case ModelDafang, ModelXiaofang:
		var q, a byte
		if quality == "sd" {
			q = 1 // 0 - hd, 1 - sd, default - hd
		}
		if audio != "0" {
			a = 1 // 0 - off, 1 - on, default - on
		}

		return errors.Join(
			c.WriteCommand(dafangVideoQuality(q)),
			c.WriteCommand(dafangVideoStart(1, a)),
		)
	}

	// 0 - auto, 1 - sd, 2 - hd, default - hd
	switch quality {
	case "", "hd":
		// Some models have broken codec settings in quality 3.
		// Some models have low quality in quality 2.
		// Different models require different default quality settings.
		switch c.model {
		case ModelC200, ModelC300:
			quality = "3"
		default:
			quality = "2"
		}
	case "sd":
		quality = "1"
	case "auto":
		quality = "0"
	}

	if audio == "" {
		audio = "1"
	}

	// Doorbell hack: keep audio enabled (doorbell refuses without it),
	// force quality=1 (SD) since doorbell 2 only sends video in SD mode.
	// Send on CS2 channel 0 (matching Mi Home app behavior from pcap).
	if c.model == ModelDoorbell2 {
		fmt.Printf("[xiaomi-debug] StartMedia for doorbell: channel=%s quality=%s audio=%s -> adjusting quality, forcing audio=1, cs2ch=0\n", channel, quality, audio)
		if quality == "1" || quality == "sd" {
			quality = "1"
		} else {
			quality = "3" // FHD 1080P 高清
		}
		audio = "1"

		data := binary.BigEndian.AppendUint32(nil, cmdVideoStart)
		data = fmt.Appendf(data, `{"videoquality":%s,"enableaudio":%s}`, quality, audio)
		fmt.Printf("[xiaomi-debug] StartMedia command (doorbell ch0): %s\n", string(data[4:]))
		return c.WriteCommandTo(0, data)
	}

	data := binary.BigEndian.AppendUint32(nil, cmdVideoStart)
	switch channel {
	case "", "0":
		data = fmt.Appendf(data, `{"videoquality":%s,"enableaudio":%s}`, quality, audio)
	default:
		data = fmt.Appendf(data, `{"videoquality":-1,"videoquality2":%s,"enableaudio":%s}`, quality, audio)
	}
	fmt.Printf("[xiaomi-debug] StartMedia command: %s\n", string(data[4:]))
	return c.WriteCommand(data)
}

func (c *Client) StopMedia() error {
	data := binary.BigEndian.AppendUint32(nil, cmdVideoStop)
	return c.WriteCommand(data)
}

func (c *Client) StartAudio() error {
	data := binary.BigEndian.AppendUint32(nil, cmdAudioStart)
	return c.WriteCommand(data)
}

func (c *Client) StartSpeaker() error {
	data := binary.BigEndian.AppendUint32(nil, cmdSpeakerStartReq)
	return c.WriteCommand(data)
}

// SpeakerCodec if the camera model has a non-standard two-way codec.
func (c *Client) SpeakerCodec() uint32 {
	switch c.model {
	case ModelDafang, ModelXiaofang, "isa.camera.hlc6":
		return codecPCM
	case "chuangmi.camera.72ac1":
		return codecOPUS
	}
	return 0
}

const hdrSize = 32

func (c *Client) ReadPacket() (*Packet, error) {
	hdr, payload, err := c.Conn.ReadPacket()
	if err != nil {
		return nil, fmt.Errorf("miss: read media: %w", err)
	}

	if len(hdr) < hdrSize {
		return nil, fmt.Errorf("miss: packet header too small")
	}

	payload, err = crypto.Decode(payload, c.key)
	if err != nil {
		return nil, err
	}

	pkt := &Packet{
		CodecID:  binary.LittleEndian.Uint32(hdr[4:]),
		Sequence: binary.LittleEndian.Uint32(hdr[8:]),
		Flags:    binary.LittleEndian.Uint32(hdr[12:]),
		Payload:  payload,
	}

	switch c.model {
	case ModelDafang, ModelXiaofang, ModelLoockV2:
		// Dafang has ts in sec
		// LoockV2 has ts in msec for video, but zero ts for audio
		pkt.Timestamp = uint64(time.Now().UnixMilli())
	default:
		pkt.Timestamp = binary.LittleEndian.Uint64(hdr[16:])
	}

	return pkt, nil
}

func (c *Client) WriteAudio(codecID uint32, payload []byte) error {
	payload, err := crypto.Encode(payload, c.key) // new payload will have new size!
	if err != nil {
		return err
	}

	n := uint32(len(payload))

	header := make([]byte, hdrSize)
	binary.LittleEndian.PutUint32(header, n)
	binary.LittleEndian.PutUint32(header[4:], codecID)
	binary.LittleEndian.PutUint64(header[16:], uint64(time.Now().UnixMilli())) // not really necessary
	return c.Conn.WritePacket(header, payload)
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

func (p *Packet) SampleRate() uint32 {
	// flag:         1 0011 000 - sample rate 16000
	// flag: 100 00 01 0000 000 - sample rate  8000
	v := (p.Flags >> 3) & 0b1111
	if v != 0 {
		return 16000
	}
	return 8000
}

//func (p *Packet) AudioUnknown1() byte {
//	return byte((p.Flags >> 7) & 0b11)
//}
//
//func (p *Packet) AudioUnknown2() byte {
//	return byte((p.Flags >> 9) & 0b11)
//}

func dafangRaw(cmd uint32, args ...byte) []byte {
	payload := tutk.ICAM(cmd, args...)

	data := make([]byte, 4+len(payload)*2)
	copy(data, "\x7f\xff\xff\xff")
	hex.Encode(data[4:], payload)
	return data
}

// DafangVideoQuality 0 - hd, 1 - sd
func dafangVideoQuality(quality uint8) []byte {
	return dafangRaw(0xff07d5, quality)
}

func dafangVideoStart(video, audio uint8) []byte {
	return dafangRaw(0xff07d8, video, audio)
}

//func dafangLeft() []byte {
//	return dafangRaw(0xff2404, 2, 0, 5)
//}
//
//func dafangRight() []byte {
//	return dafangRaw(0xff2404, 1, 0, 5)
//}
//
//func dafangUp() []byte {
//	return dafangRaw(0xff2404, 0, 2, 5)
//}
//
//func dafangDown() []byte {
//	return dafangRaw(0xff2404, 0, 1, 5)
//}
//
//func dafangStop() []byte {
//	return dafangRaw(0xff2404, 0, 0, 5)
//}
