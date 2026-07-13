package legacy

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

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
	rawMode := query.Get("xraw")
	var localAddr *net.UDPAddr

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

	if port := query.Get("xport"); port != "" {
		// Experimental: force direct host port from URL query.
		if host := u.Hostname(); net.ParseIP(host) != nil {
			u.Host = net.JoinHostPort(host, port)
		}
	} else if model == ModelLoockV1 && query.Get("xdirect") == "1" {
		// Experimental CatY mode based on captured Mi Home LAN traffic.
		if host := u.Hostname(); net.ParseIP(host) != nil {
			u.Host = net.JoinHostPort(host, "6666")
		}
	}
	if model == ModelLoockV1 && rawMode != "" && rawMode != "3" {
		// Experimental: replay a small subset of observed Mi Home UDP payloads.
		// This is best-effort and intentionally ignored on error.
		_ = loockRawKick(u.Host, query.Get("xlocal"), rawMode)
	}
	if localPort := query.Get("xlocal"); localPort != "" {
		port, err := strconv.Atoi(localPort)
		if err != nil {
			return nil, fmt.Errorf("xiaomi: invalid xlocal: %w", err)
		}
		localAddr = &net.UDPAddr{Port: port}
	}

	cfg := &tutk.DialConfig{LocalAddr: localAddr}
	if model == ModelLoockV1 && rawMode == "3" {
		cfg.PreConnect = func(conn *net.UDPConn, addr *net.UDPAddr) error {
			return loockRawKickConn(conn, addr, rawMode)
		}
	}

	conn, err := tutk.DialWithConfig(u.Host, query.Get("uid"), username, password, cfg)
	if err != nil {
		return nil, err
	}

	if model == ModelDafang || model == ModelXiaofang || (model == ModelLoockV1 && query.Get("xskiplogin") != "1") {
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

func loockRawKick(hostport, localPort, mode string) error {
	host := hostport
	port := "6666"
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		host, port = h, p
	}
	if net.ParseIP(host) == nil {
		return nil
	}

	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		return err
	}

	var localAddr *net.UDPAddr
	if localPort != "" {
		p, err := strconv.Atoi(localPort)
		if err != nil {
			return err
		}
		localAddr = &net.UDPAddr{Port: p}
	}

	conn, err := net.ListenUDP("udp", localAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err = loockRawKickConn(conn, addr, mode); err != nil {
		return err
	}

	return nil
}

func loockRawKickConn(conn *net.UDPConn, addr *net.UDPAddr, mode string) error {
	payloadHex, loops, delay := loockRawPayload(mode)

	_ = conn.SetDeadline(time.Now().Add(200 * time.Millisecond))
	for i := 0; i < loops; i++ {
		for _, s := range payloadHex {
			b, err := hex.DecodeString(s)
			if err != nil {
				continue
			}
			_, _ = conn.WriteToUDP(b, addr)

			buf := make([]byte, 2048)
			_, _, _ = conn.ReadFromUDP(buf) // ignore result, this is just a wake/kick attempt
			time.Sleep(delay)
		}
	}
	return nil
}

func loockRawPayload(mode string) (payloadHex []string, loops int, delay time.Duration) {
	// Payloads captured from Mi Home <-> CatY local LAN session.
	// They are protocol-ciphertext and may vary by session/device state.
	switch mode {
	case "2", "3":
		// Replay a longer startup burst captured from phone traffic.
		payloadHex = []string{
			"6e4c9d8c40d140ca3d2da82dc0e6cadcfb4bde8b775484ae0ef4ab8815d1af5c6e2e8d8c40d040ca3e6d3b1f40a4cbd8637f06e9a741f6d72d6e280c30e4fad86e2e8d8c40d040ca2d4d280c40e4cad8206c726168656943",
			"6e6c5df840db30cb3d2da82d20eecafcf7729d2c306140ca8dbd280c3fe5ba946e2ead8e40c060ca2d6d280c40e4cad8e8f8dba72386b65d0a3dc573f6e59dff481dab8f799732cd0e5a0aae0782f8ce685dfbd972d307f80e292b1f73d2accf6d18fb0d24f777e85e2e1b2a52804cff78a8beda2793d3c90e7abe6f67d199da48ccabee7ab767f81e3ebb0f86c65dfa6878fb8920a707a85b7c3b5f4685a8fa3808abcd27c703891e0cbe4f0783acaa1d5dbeb4239342bd0e3a2e7f73dc88df18adeece721352e80e3f1b8a57d1d9fa6d08ff25243692ec5b791e3a128bed3e6e2e8d8c40d040ca2dad2f0c40e4cbd86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad84dadc9183ac517386d7c280c47e4d858dab9eaf81592d3be195c4dafa7d2ed4ddd6bebab32a5c71d0860bb1b44b7bd2e6e2e8d8c40d040ca4b4c1b0b518bcd7c6e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad82e2e8d8c406041b42d6c280c40e4cad86e2e8d8c40d070ca2d8d290c40e4cbd8436861726c696e6c5df840db30cb3d2ca82d20eecafcf7729d2c306340ca8dbd280c3fe5ba946e2ead8e40d060ca2d6d280c40e4cad8e8f8dba72386b65d0a1dc573f6e59cff481dab8f799732cd0e5a0aae0782f8ce685dfbd972d307f80e292b1f73d2accf6d18fb0d24f777e85e2e1b2a52804cff78a8beda2793d3c90e7abe6f67d199da48ccabee7ab767f81e3ebb0f86c65dfa6878fb8920a707a85b7c3b5f4685a8fa3808abcd27c703891e0cbe4f0783acaa1d5dbeb4239342bd0e3a2e7f73dc88df18adeece721352e80e3f1b8a57d1d9fa6d08ff25243692ec5b791e3a128bed3e6e2e8d8c40d040ca2dad2f0c40e4cbd86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad84dadc9183ac517386d7c280c47e4d858dab9eaf81592d3be195c4dafa7d2ed4ddd6bebab32a5c71d0860bb1b44b7bd2e6e2e8d8c40d040ca4b4c1b0b518bcd7c6e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad86e2e8d8c40d040ca2d6d280c40e4cad82e2e8d8c406041b42d6c280c40e4cad86e2e8d8c40d070ca2d8d290c40e4cbd8436861726c69",
			"4e6d9d8c40d140ca3d2da82d00e6cadafb4bde8b775484ae0ef4ab8815d1af5cf7729d2c30d140ca9e7d3b1f3fa5bb94627144684e6d9d8c40d140ca3d2da82d00e6cadafb4bde8b775484ae0ef4ab8815d1af5cf7729d2c30d140ca9e7d3b1f3fa5bb9462714468",
			"4e6d9d8c40d140ca3d2da82d00e6cadafb4bde8b775484ae0ef4ab8815d1af5cf7729d2c30d140ca9e7d3b1f3fa5bb9462714468",
		}
		loops = 2
		delay = 10 * time.Millisecond
	default:
		payloadHex = []string{
			"6e4c9d8c40d140ca3d2da82dc0e6cadcfb4bde8b775484ae0ef4ab8815d1af5c6e2e8d8c40d040ca3e6d3b1f40a4cbd8637f06e9a741f6d72d6e280c30e4fad86e2e8d8c40d040ca2d4d280c40e4cad8206c726168656943",
			"4e6d9d8c40d140ca3d2da82d00e6cadafb4bde8b775484ae0ef4ab8815d1af5cf7729d2c30d140ca9e7d3b1f3fa5bb9462714468",
		}
		loops = 3
		delay = 15 * time.Millisecond
	}

	return
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

	case ModelLoockV1:
		// CatY firmware variants behave differently.
		// Send a wide set of known-safe start commands and ignore partial failures.
		switch video {
		case "", "hd":
			video = "3"
		case "sd":
			video = "1"
		case "auto":
			video = "0"
		}

		_ = c.WriteCommandJSON(cmdAudioStart, `{}`)
		_ = c.WriteCommandJSON(cmdVideoStart, `{}`)
		_ = c.WriteCommandJSON(cmdStreamCtrlReq, `{"videoquality":%s}`, video)
		_ = c.WriteCommandJSON(0x0605, `{"channel":1}`)
		_ = c.WriteCommandJSON(0x0704, `{}`)
		return nil

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
