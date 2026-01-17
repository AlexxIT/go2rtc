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
	} else if model == ModelXiaobai {
		username = "admin"
		password = query.Get("password")
	} else if model == ModelXiaofang {
		username = "admin"
	} else {
		return nil, fmt.Errorf("xiaomi: unsupported model: %s", model)
	}

	conn, err := tutk.Dial(u.Host, query.Get("uid"), username, password)
	if err != nil {
		return nil, err
	}

	if model == ModelXiaofang {
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
	if err != nil {
		return err
	}
	return nil
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
		switch hdr[0] {
		case tutk.CodecH264, tutk.CodecH265:
			payload, err = DecodeVideo(payload, c.key)
			if err != nil {
				return
			}
		case tutk.CodecAAC:
			payload, err = crypto.Decode(payload, c.key)
			if err != nil {
				return
			}
		}
	}
	return
}

func (c *Client) StartMedia(video, audio string) error {
	switch c.model {
	case ModelAqaraG2:
		return c.WriteCommand(0x01ff, []byte(`{}`))

	case ModelXiaobai:
		// 00030000 7b7d  audio on
		// 01030000 7b7d  audio off
		if err := c.WriteCommand(0x0300, []byte(`{}`)); err != nil {
			return err
		}

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
		// 20030000 0000000001000000  fhd (1920x1080)
		// 20030000 0000000002000000  hd (1280x720)
		// 20030000 0000000004000000  low (640x360)
		// 20030000 00000000ff000000  auto (1920x1080)
		if err := c.WriteCommand(0x0320, []byte{0, 0, 0, 0, b, 0, 0, 0}); err != nil {
			return err
		}

		// ff010000 7b7d  video tart
		// ff020000 7b7d  video stop
		return c.WriteCommand(0x01ff, []byte(`{}`))

	case ModelXiaofang:
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
	}

	return nil
}

func (c *Client) StopMedia() error {
	return errors.Join(
		c.WriteCommand(0x02ff, []byte(`{}`)),
		c.WriteCommand(0x02ff, make([]byte, 8)),
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
	i1 := binary.LittleEndian.Uint16(data[9:])
	i2 := binary.LittleEndian.Uint16(data[13:])
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
	ModelLoockV1  = "loock.cateye.v01"
	ModelXiaobai  = "chuangmi.camera.xiaobai"
	ModelXiaofang = "isa.camera.isc5"
)

func Supported(model string) bool {
	switch model {
	case ModelAqaraG2, ModelLoockV1, ModelXiaobai, ModelXiaofang:
		return true
	}
	return false
}
