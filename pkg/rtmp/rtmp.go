package rtmp

import (
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
)

const (
	MsgSetPacketSize   = 1
	MsgServerBandwidth = 5
	MsgClientBandwidth = 6
	MsgCommand         = 20

	//MsgAck         = 3
	//MsgControl     = 4
	//MsgAudioPacket = 8
	//MsgVideoPacket = 9
	//MsgDataExt     = 15
	//MsgCommandExt  = 17
	//MsgData        = 18
)

var ErrResponse = errors.New("rtmp: wrong response")

// rtmp - implements flv.Transport
type rtmp struct {
	url     string
	app     string
	stream  string
	pktSize uint32

	headers map[uint32]*header

	conn net.Conn
	rd   io.Reader
}

type header struct {
	msgTime uint32
	msgSize uint32
	msgType byte
}

func (c *rtmp) ReadTag() (byte, uint32, []byte, error) {
	hdrType, sid, err := c.readHeader()
	if err != nil {
		return 0, 0, nil, err
	}

	// storing header information for support header type 3
	hdr, ok := c.headers[sid]
	if !ok {
		hdr = &header{}
		c.headers[sid] = hdr
	}

	var b []byte

	// https://en.wikipedia.org/wiki/Real-Time_Messaging_Protocol#Packet_structure
	switch hdrType {
	case 0: // 12 byte header (full header)
		if b, err = c.readSize(11); err != nil {
			return 0, 0, nil, err
		}
		_ = b[7]
		hdr.msgTime = Uint24(b)            // timestamp
		hdr.msgSize = Uint24(b[3:])        // msgdatalen
		hdr.msgType = b[6]                 // msgtypeid
		_ = binary.BigEndian.Uint32(b[7:]) // msgsid

	case 1: // 8 bytes - like type b00, not including message ID (4 last bytes)
		if b, err = c.readSize(7); err != nil {
			return 0, 0, nil, err
		}
		_ = b[6]
		hdr.msgTime = Uint24(b)     // timestamp
		hdr.msgSize = Uint24(b[3:]) // msgdatalen
		hdr.msgType = b[6]          // msgtypeid

	case 2: // 4 bytes - Basic Header and timestamp (3 bytes) are included
		if b, err = c.readSize(3); err != nil {
			return 0, 0, nil, err
		}
		hdr.msgTime = Uint24(b) // timestamp

	case 3: // 1 byte - only the Basic Header is included
		// use here hdr from previous msg with same session ID (sid)
	}

	timeMS := hdr.msgTime
	if timeMS == 0xFFFFFF {
		if b, err = c.readSize(4); err != nil {
			return 0, 0, nil, err
		}
		timeMS = binary.BigEndian.Uint32(b)
	}

	//log.Printf("[rtmp] hdrType=%d sid=%d msdTime=%d msgSize=%d msgType=%d", hdrType, sid, hdr.msgTime, hdr.msgSize, hdr.msgType)

	// 1. Response zero size
	if hdr.msgSize == 0 {
		return hdr.msgType, timeMS, nil, nil
	}

	b = make([]byte, hdr.msgSize)

	// 2. Response small packet
	if c.pktSize == 0 || hdr.msgSize < c.pktSize {
		if _, err = io.ReadFull(c.rd, b); err != nil {
			return 0, 0, nil, err
		}
		return hdr.msgType, timeMS, b, nil
	}

	// 3. Response big packet
	var i0 uint32
	for i1 := c.pktSize; i1 < hdr.msgSize; i1 += c.pktSize {
		if _, err = io.ReadFull(c.rd, b[i0:i1]); err != nil {
			return 0, 0, nil, err
		}

		if _, _, err = c.readHeader(); err != nil {
			return 0, 0, nil, err
		}

		if hdr.msgTime == 0xFFFFFF {
			if _, err = c.readSize(4); err != nil {
				return 0, 0, nil, err
			}
		}

		i0 = i1
	}

	if _, err = io.ReadFull(c.rd, b[i0:]); err != nil {
		return 0, 0, nil, err
	}

	return hdr.msgType, timeMS, b, nil
}

func (c *rtmp) Close() error {
	return c.conn.Close()
}

func (c *rtmp) init() error {
	if err := c.handshake(); err != nil {
		return err
	}
	if err := c.sendConfig(); err != nil {
		return err
	}

	c.headers = map[uint32]*header{}

	if err := c.sendConnect(); err != nil {
		return err
	}
	if err := c.sendPlay(); err != nil {
		return err
	}

	return nil
}

func (c *rtmp) handshake() error {
	// simple handshake without real random and check response
	const randomSize = 4 + 4 + 1528

	b := make([]byte, 1+randomSize)
	b[0] = 0x03
	if _, err := c.conn.Write(b); err != nil {
		return err
	}

	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}

	if b[0] != 3 {
		return errors.New("rtmp: wrong handshake")
	}

	if _, err := c.conn.Write(b[1:]); err != nil {
		return err
	}

	if _, err := io.ReadFull(c.rd, b[1:]); err != nil {
		return err
	}

	return nil
}

func (c *rtmp) sendConfig() error {
	b := make([]byte, 5)
	binary.BigEndian.PutUint32(b, 65536)
	if err := c.sendRequest(MsgSetPacketSize, 0, b[:4]); err != nil {
		return err
	}

	binary.BigEndian.PutUint32(b, 2500000)
	if err := c.sendRequest(MsgServerBandwidth, 0, b[:4]); err != nil {
		return err
	}

	binary.BigEndian.PutUint32(b, 10000000) // ack size
	b[4] = 2                                // limit type
	if err := c.sendRequest(MsgClientBandwidth, 0, b); err != nil {
		return err
	}

	return nil
}

func (c *rtmp) sendConnect() error {
	msg := amf.AMF{}
	msg.WriteString("connect")
	msg.WriteNumber(1)
	msg.WriteObject(map[string]any{
		"app":           c.app,
		"flashVer":      "MAC 32,0,0,0",
		"tcUrl":         c.url,
		"fpad":          false, // ?
		"capabilities":  15,    // ?
		"audioCodecs":   4071,  // ?
		"videoCodecs":   252,   // ?
		"videoFunction": 1,     // ?
	})

	if err := c.sendRequest(MsgCommand, 0, msg.Bytes()); err != nil {
		return err
	}

	s, err := c.waitCode()
	if err != nil {
		return err
	}

	if s != "NetConnection.Connect.Success" {
		return errors.New("rtmp: wrong code: " + s)
	}

	return nil
}

func (c *rtmp) sendPlay() error {
	msg := amf.NewWriter()
	msg.WriteString("createStream")
	msg.WriteNumber(2)
	msg.WriteNull()

	if err := c.sendRequest(MsgCommand, 0, msg.Bytes()); err != nil {
		return err
	}

	args, err := c.waitResponse()
	if err != nil {
		return err
	}

	if len(args) < 4 {
		return ErrResponse
	}

	sid, _ := args[3].(float64)

	msg = amf.NewWriter()
	msg.WriteString("play")
	msg.WriteNumber(3)
	msg.WriteNull()
	msg.WriteString(c.stream)

	if err = c.sendRequest(MsgCommand, uint32(sid), msg.Bytes()); err != nil {
		return err
	}

	s, err := c.waitCode()
	if err != nil {
		return err
	}

	switch s {
	case "NetStream.Play.Start", "NetStream.Play.Reset":
		return nil
	}

	return errors.New("rtmp: wrong code: " + s)
}

func (c *rtmp) sendRequest(msgType byte, streamID uint32, payload []byte) error {
	n := len(payload)
	b := make([]byte, 12+n)
	_ = b[12]

	switch msgType {
	case MsgSetPacketSize, MsgServerBandwidth, MsgClientBandwidth:
		b[0] = 0x02 // chunk ID
	case MsgCommand:
		if streamID == 0 {
			b[0] = 0x03 // chunk ID
		} else {
			b[0] = 0x08 // chunk ID
		}
	}

	//PutUint24(b[1:], 0)                  // timestamp
	PutUint24(b[4:], uint32(n))                 // payload size
	b[7] = msgType                              // message type
	binary.BigEndian.PutUint32(b[8:], streamID) // message stream ID
	copy(b[12:], payload)

	if _, err := c.conn.Write(b); err != nil {
		return err
	}

	return nil
}

func (c *rtmp) readHeader() (byte, uint32, error) {
	b, err := c.readSize(1)
	if err != nil {
		return 0, 0, err
	}

	hdrType := b[0] >> 6
	sid := uint32(b[0] & 0b111111)

	switch sid {
	case 0:
		if b, err = c.readSize(1); err != nil {
			return 0, 0, err
		}
		sid = 64 + uint32(b[0])
	case 1:
		if b, err = c.readSize(2); err != nil {
			return 0, 0, err
		}
		sid = 64 + uint32(binary.BigEndian.Uint16(b))
	}

	return hdrType, sid, nil
}

func (c *rtmp) readSize(n uint32) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadAtLeast(c.rd, b, int(n)); err != nil {
		return nil, err
	}
	return b, nil
}

func (c *rtmp) waitResponse() ([]any, error) {
	for {
		msgType, _, b, err := c.ReadTag()
		if err != nil {
			return nil, err
		}

		switch msgType {
		case MsgSetPacketSize:
			c.pktSize = binary.BigEndian.Uint32(b)

		case MsgCommand:
			return amf.NewReader(b).ReadItems()
		}
	}
}

func (c *rtmp) waitCode() (string, error) {
	args, err := c.waitResponse()
	if err != nil {
		return "", err
	}

	if len(args) < 4 {
		return "", ErrResponse
	}

	m, _ := args[3].(map[string]any)
	if m == nil {
		return "", ErrResponse
	}

	v, _ := m["code"]
	if v == nil {
		return "", ErrResponse
	}

	s, _ := v.(string)
	return s, nil
}

func PutUint24(b []byte, v uint32) {
	_ = b[2]
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
}

func Uint24(b []byte) uint32 {
	_ = b[2]
	return uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
}
