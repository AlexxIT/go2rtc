package rtmp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
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

type Reader struct {
	url     string
	app     string
	stream  string
	pktSize uint32

	headers map[uint32]*header

	conn net.Conn
	rd   io.Reader

	buf []byte
}

func NewReader(u *url.URL, conn net.Conn) (*Reader, error) {
	rd := &Reader{
		url:     u.String(),
		headers: map[uint32]*header{},
		conn:    conn,
		rd:      bufio.NewReaderSize(conn, core.BufferSize),
	}

	if args := strings.Split(u.Path, "/"); len(args) >= 2 {
		rd.app = args[1]
		if len(args) >= 3 {
			rd.stream = args[2]
			if u.RawQuery != "" {
				rd.stream += "?" + u.RawQuery
			}
		}
	}

	if err := rd.handshake(); err != nil {
		return nil, err
	}
	if err := rd.sendConfig(); err != nil {
		return nil, err
	}
	if err := rd.sendConnect(); err != nil {
		return nil, err
	}
	if err := rd.sendPlay(); err != nil {
		return nil, err
	}

	rd.buf = []byte{
		'F', 'L', 'V', // signature
		1,          // version
		0,          // flags (has video/audio)
		0, 0, 0, 9, // header size
	}

	return rd, nil
}

func (c *Reader) Read(p []byte) (n int, err error) {
	// 1. Check temporary tempbuffer
	if len(c.buf) == 0 {
		msgType, timeMS, payload, err2 := c.readMessage()
		if err2 != nil {
			return 0, err2
		}

		payloadSize := len(payload)

		// previous tag size (4 byte) + header (11 byte) + payload
		n = 4 + 11 + payloadSize

		// 2. Check if the message fits in the buffer
		if n <= len(p) {
			encodeFLV(p, msgType, timeMS, uint32(payloadSize), payload)
			return
		}

		// 3. Put the message into a temporary buffer
		c.buf = make([]byte, n)
		encodeFLV(c.buf, msgType, timeMS, uint32(payloadSize), payload)
	}

	// 4. Send temporary buffer
	n = copy(p, c.buf)
	c.buf = c.buf[n:]
	return
}

func (c *Reader) Close() error {
	return c.conn.Close()
}

func encodeFLV(b []byte, msgType byte, time, size uint32, payload []byte) {
	b[0] = 0
	b[1] = 0
	b[2] = 0
	b[3] = 0
	b[4+0] = msgType
	PutUint24(b[4+1:], size)
	PutUint24(b[4+4:], time)
	b[4+7] = byte(time >> 24)

	copy(b[4+11:], payload)
}

type header struct {
	msgTime uint32
	msgSize uint32
	msgType byte
}

func (c *Reader) readMessage() (byte, uint32, []byte, error) {
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

	//log.Printf("[Reader] hdrType=%d sid=%d msdTime=%d msgSize=%d msgType=%d", hdrType, sid, hdr.msgTime, hdr.msgSize, hdr.msgType)

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

func (c *Reader) handshake() error {
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
		return errors.New("Reader: wrong handshake")
	}

	if _, err := c.conn.Write(b[1:]); err != nil {
		return err
	}

	if _, err := io.ReadFull(c.rd, b[1:]); err != nil {
		return err
	}

	return nil
}

func (c *Reader) sendConfig() error {
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

func (c *Reader) sendConnect() error {
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

	s, err := c.waitCode("_result", float64(1)) // result with same ID
	if err != nil {
		return err
	}

	if s != "NetConnection.Connect.Success" {
		return errors.New("Reader: wrong code: " + s)
	}

	return nil
}

func (c *Reader) sendPlay() error {
	msg := amf.NewWriter()
	msg.WriteString("createStream")
	msg.WriteNumber(2)
	msg.WriteNull()

	if err := c.sendRequest(MsgCommand, 0, msg.Bytes()); err != nil {
		return err
	}

	args, err := c.waitResponse("_result", float64(2)) // result with same ID
	if err != nil {
		return err
	}

	if len(args) < 4 {
		return ErrResponse
	}

	sid, _ := args[3].(float64)

	msg = amf.NewWriter()
	msg.WriteString("play")
	msg.WriteNumber(0)
	msg.WriteNull()
	msg.WriteString(c.stream)

	if err = c.sendRequest(MsgCommand, uint32(sid), msg.Bytes()); err != nil {
		return err
	}

	s, err := c.waitCode("onStatus", float64(0)) // events has zero transaction ID
	if err != nil {
		return err
	}

	switch s {
	case "NetStream.Play.Start", "NetStream.Play.Reset":
		return nil
	}

	return errors.New("Reader: wrong code: " + s)
}

func (c *Reader) sendRequest(msgType byte, streamID uint32, payload []byte) error {
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

func (c *Reader) readHeader() (byte, uint32, error) {
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

func (c *Reader) readSize(n uint32) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadAtLeast(c.rd, b, int(n)); err != nil {
		return nil, err
	}
	return b, nil
}

func (c *Reader) waitResponse(cmd any, tid any) ([]any, error) {
	for {
		msgType, _, b, err := c.readMessage()
		if err != nil {
			return nil, err
		}

		switch msgType {
		case MsgSetPacketSize:
			c.pktSize = binary.BigEndian.Uint32(b)

		case MsgCommand:
			var v []any
			if v, err = amf.NewReader(b).ReadItems(); err != nil {
				return nil, err
			}

			if len(v) < 4 {
				return nil, ErrResponse
			}

			if v[0] == cmd && v[1] == tid {
				return v, nil
			}
		}
	}
}

func (c *Reader) waitCode(cmd any, tid any) (string, error) {
	args, err := c.waitResponse(cmd, tid)
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
