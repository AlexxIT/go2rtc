package legacy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/tutk"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/crypto"
)

func NewClient(rawURL string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	query := u.Query()
	model := query.Get("model")

	var username, password string
	var key []byte

	if query.Has("sign") {
		// Legacy with encryption
		key, err = crypto.CalcSharedKey(query.Get("device_public"), query.Get("client_private"))
		if err != nil {
			return nil, err
		}

		username = fmt.Sprintf(
			`{"public_key":"%s","sign":"%s","account":"admin"}`,
			query.Get("client_public"), query.Get("sign"),
		)
	} else if model == ModelMijia || model == ModelXiaobai {
		username = "admin"
		password = query.Get("password")
	} else if model == ModelDafang || model == ModelXiaofang {
		username = "admin"
	} else {
		return nil, fmt.Errorf("xiaomi: unsupported model: %s", model)
	}

	conn, err := tutk.Dial(u.Host, query.Get("uid"), username, password)
	if err != nil {
		return nil, err
	}

	if model == ModelDafang || model == ModelXiaofang {
		err = xiaofangLogin(conn, query.Get("password"))
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
	}

	c := &Client{
		Conn:  conn,
		key:   key,
		model: model,
	}

	return c, nil
}

func xiaofangLogin(conn *tutk.Conn, password string) error {
	data := tutk.ICAM(0x0400be) // ask login
	if err := conn.WriteCommand(0x0100, data); err != nil {
		return err
	}

	_, data, err := conn.ReadCommand() // login request
	if err != nil {
		return err
	}

	enc := data[24:] // data[23] == 3
	tutk.XXTEADecrypt(enc, enc, []byte(password))

	enc = append(enc, 0, 0, 0, 0, 1, 1, 1)
	data = tutk.ICAM(0x0400c0, enc...) // login response
	if err = conn.WriteCommand(0x0100, data); err != nil {
		return err
	}

	_, data, err = conn.ReadCommand()
	return err
}

type Client struct {
	*tutk.Conn
	key   []byte
	model string
}

func (c *Client) Version() string {
	return fmt.Sprintf("%s (%s)", c.Conn.Version(), c.model)
}

func (c *Client) ReadPacket() (hdr, payload []byte, err error) {
	hdr, payload, err = c.Conn.ReadPacket()
	if err != nil {
		return
	}
	if c.key != nil {
		if c.model == ModelAqaraG2 && hdr[0] == tutk.CodecH265 {
			payload, err = DecodeVideo(payload, c.key)
		} else {
			// ModelAqaraG2: audio AAC
			// ModelIMILABA1: video HEVC, audio PCMA
			payload, err = crypto.Decode(payload, c.key)
		}
	}
	return
}

const (
	cmdVideoStart    = 0x01ff
	cmdVideoStop     = 0x02ff
	cmdAudioStart    = 0x0300
	cmdAudioStop     = 0x0301
	cmdStreamCtrlReq = 0x0320
)

func (c *Client) WriteCommandJSON(ctrlType uint32, format string, a ...any) error {
	if len(a) > 0 {
		format = fmt.Sprintf(format, a...)
	}
	return c.WriteCommand(ctrlType, []byte(format))
}

func (c *Client) StartMedia(video, audio string) error {
	switch c.model {
	case ModelAqaraG2:
		// 0 - 1920x1080, 1 - 1280x720, 2 - ?
		switch video {
		case "", "fhd":
			video = "0"
		case "hd":
			video = "1"
		case "sd":
			video = "2"
		}

		return errors.Join(
			c.WriteCommandJSON(cmdVideoStart, `{}`),
			c.WriteCommandJSON(0x0605, `{"channel":%s}`, video),
			c.WriteCommandJSON(0x0704, `{}`), // don't know why
		)

	case ModelIMILABA1, ModelMijia:
		// 0 - auto, 1 - low, 3 - hd
		switch video {
		case "", "hd":
			video = "3"
		case "sd":
			video = "1" // 2 is also low quality
		case "auto":
			video = "0"
		}

		// quality after start
		return errors.Join(
			c.WriteCommandJSON(cmdAudioStart, `{}`),
			c.WriteCommandJSON(cmdVideoStart, `{}`),
			c.WriteCommandJSON(cmdStreamCtrlReq, `{"videoquality":%s}`, video),
		)

	case ModelXiaobai:
		// 00030000 7b7d  audio on
		// 01030000 7b7d  audio off
		// 20030000 0000000001000000  fhd (1920x1080)
		// 20030000 0000000002000000  hd (1280x720)
		// 20030000 0000000004000000  low (640x360)
		// 20030000 00000000ff000000  auto (1920x1080)
		// ff010000 7b7d  video tart
		// ff020000 7b7d  video stop

		var b byte
		switch video {
		case "", "fhd":
			b = 1
		case "hd":
			b = 2
		case "sd":
			b = 4
		case "auto":
			b = 0xff
		}

		// quality before start
		return errors.Join(
			c.WriteCommandJSON(cmdAudioStart, `{}`),
			c.WriteCommand(cmdStreamCtrlReq, []byte{0, 0, 0, 0, b, 0, 0, 0}),
			c.WriteCommandJSON(cmdVideoStart, `{}`),
		)

	case ModelDafang, ModelXiaofang:
		// 00010000 4943414d 95010400000000000000000600000000000000d20400005a07 - 90k bitrate
		// 00010000 4943414d 95010400000000000000000600000000000000d20400001e07 - 30k bitrate
		//var b byte
		//switch video {
		//case "", "hd":
		//	b = 0x5a // bitrate 90k
		//case "sd":
		//	b = 0x1e // bitrate 30k
		//}
		//data := tutk.ICAM(0x040195, 0xd2, 4, 0, 0, b, 7)
		//if err := c.WriteCommand(0x100, data); err != nil {
		//	return err
		//}
		return nil
	}

	return fmt.Errorf("xiaomi: unsupported model: %s", c.model)
}

func (c *Client) StopMedia() error {
	return errors.Join(
		c.WriteCommandJSON(cmdVideoStop, `{}`),
		c.WriteCommand(cmdVideoStop, make([]byte, 8)),
	)
}

func DecodeVideo(data, key []byte) ([]byte, error) {
	if string(data[:4]) == "\x00\x00\x00\x01" || data[8] == 0 {
		return data, nil
	}

	if data[8] != 1 {
		// Support could be added, but I haven't seen such cameras.
		return nil, fmt.Errorf("xiaomi: unsupported encryption")
	}

	nonce8 := data[:8]
	i1 := binary.LittleEndian.Uint32(data[9:])
	i2 := binary.LittleEndian.Uint32(data[13:])
	data = data[17:]
	src := data[i1 : i1+i2]

	for i := 32; i+16 < len(src); i += 160 {
		dst, err := crypto.DecodeNonce(src[i:i+16], nonce8, key)
		if err != nil {
			return nil, err
		}
		copy(src[i:], dst) // copy result in same place
	}

	return data, nil
}

const (
	ModelAqaraG2  = "lumi.camera.gwagl01"
	ModelIMILABA1 = "chuangmi.camera.ipc019e"
	ModelLoockV1  = "loock.cateye.v01"
	ModelXiaobai  = "chuangmi.camera.xiaobai"
	ModelXiaofang = "isa.camera.isc5"
	// ModelMijia support miss format for new fw and legacy format for old fw
	ModelMijia = "chuangmi.camera.v2"
	// ModelDafang support miss format for new fw and legacy format for old fw
	ModelDafang = "isa.camera.df3"
)

func Supported(model string) bool {
	switch model {
	case ModelAqaraG2, ModelIMILABA1, ModelLoockV1, ModelXiaobai, ModelXiaofang:
		return true
	}
	return false
}
