package rtmp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
)

const (
	TypeSetPacketSize   = 1
	TypeServerBandwidth = 5
	TypeClientBandwidth = 6
	TypeAudio           = 8
	TypeVideo           = 9
	TypeData            = 18
	TypeCommand         = 20
)

type Conn struct {
	App    string
	Stream string
	Intent string

	rdPacketSize uint32
	wrPacketSize uint32

	chunks   map[byte]*header
	streamID byte
	url      string

	conn net.Conn
	rd   io.Reader
	wr   io.Writer

	rdBuf []byte
	wrBuf []byte
	mu    sync.Mutex
}

func (c *Conn) Close() error {
	return c.conn.Close()
}

func (c *Conn) readResponse(transID float64) ([]any, error) {
	for {
		msgType, _, b, err := c.readMessage()
		if err != nil {
			return nil, err
		}

		switch msgType {
		case TypeSetPacketSize:
			c.rdPacketSize = binary.BigEndian.Uint32(b)
		case TypeCommand:
			items, _ := amf.NewReader(b).ReadItems()
			if len(items) >= 3 && items[1] == transID {
				return items, nil
			}
		}
	}
}

type header struct {
	timeMS   uint32
	dataSize uint32
	tagType  byte
	streamID uint32
}

//var ErrNotImplemented = errors.New("rtmp: not implemented")

func (c *Conn) readMessage() (byte, uint32, []byte, error) {
	b, err := c.readSize(1) // doesn't support big chunkID!!!
	if err != nil {
		return 0, 0, nil, err
	}

	hdrType := b[0] >> 6
	chunkID := b[0] & 0b111111

	// storing header information for support header type 3
	hdr, ok := c.chunks[chunkID]
	if !ok {
		hdr = &header{}
		c.chunks[chunkID] = hdr
	}

	switch hdrType {
	case 0: // 12 byte header (full header)
		if b, err = c.readSize(11); err != nil {
			return 0, 0, nil, err
		}
		_ = b[7]
		hdr.timeMS = Uint24(b)
		hdr.dataSize = Uint24(b[3:])
		hdr.tagType = b[6]
		hdr.streamID = binary.LittleEndian.Uint32(b[7:])

	case 1: // 8 bytes - like type b00, not including message ID (4 last bytes)
		if b, err = c.readSize(7); err != nil {
			return 0, 0, nil, err
		}
		_ = b[6]
		hdr.timeMS = Uint24(b)       // timestamp
		hdr.dataSize = Uint24(b[3:]) // msgdatalen
		hdr.tagType = b[6]           // msgtypeid

	case 2: // 4 bytes - Basic Header and timestamp (3 bytes) are included
		if b, err = c.readSize(3); err != nil {
			return 0, 0, nil, err
		}
		hdr.timeMS = Uint24(b) // timestamp

	case 3: // 1 byte - only the Basic Header is included
		// use here hdr from previous msg with same session ID (sid)
	}

	timeMS := hdr.timeMS
	if timeMS == 0xFFFFFF {
		if b, err = c.readSize(4); err != nil {
			return 0, 0, nil, err
		}
		timeMS = binary.BigEndian.Uint32(b)
	}

	//log.Printf("[rtmp] hdr=%d chunkID=%d timeMS=%d size=%d tagType=%d streamID=%d", hdrType, chunkID, hdr.timeMS, hdr.dataSize, hdr.tagType, hdr.streamID)

	// 1. Response zero size
	if hdr.dataSize == 0 {
		return hdr.tagType, timeMS, nil, nil
	}

	b = make([]byte, hdr.dataSize)

	// 2. Response small packet
	if hdr.dataSize <= c.rdPacketSize {
		if _, err = io.ReadFull(c.rd, b); err != nil {
			return 0, 0, nil, err
		}
		return hdr.tagType, timeMS, b, nil
	}

	// 3. Response big packet
	var i0 uint32
	for i1 := c.rdPacketSize; i1 < hdr.dataSize; i1 += c.rdPacketSize {
		if _, err = io.ReadFull(c.rd, b[i0:i1]); err != nil {
			return 0, 0, nil, err
		}

		if _, err = c.readSize(1); err != nil {
			return 0, 0, nil, err
		}

		if hdr.timeMS == 0xFFFFFF {
			if _, err = c.readSize(4); err != nil {
				return 0, 0, nil, err
			}
		}

		i0 = i1
	}

	if _, err = io.ReadFull(c.rd, b[i0:]); err != nil {
		return 0, 0, nil, err
	}

	return hdr.tagType, timeMS, b, nil
}
func (c *Conn) writeMessage(chunkID, tagType byte, timeMS uint32, payload []byte) error {
	c.mu.Lock()
	c.resetBuffer()

	b := payload
	size := uint32(len(b))

	if size > c.wrPacketSize {
		c.appendType0(chunkID, tagType, timeMS, size, b[:c.wrPacketSize])

		for {
			b = b[c.wrPacketSize:]
			if uint32(len(b)) > c.wrPacketSize {
				c.appendType3(chunkID, b[:c.wrPacketSize])
			} else {
				c.appendType3(chunkID, b)
				break
			}
		}
	} else {
		c.appendType0(chunkID, tagType, timeMS, size, b)
	}

	//log.Printf("%d %2d %5d %6d %.32x", chunkID, tagType, timeMS, size, payload)

	_, err := c.wr.Write(c.wrBuf)
	c.mu.Unlock()
	return err
}

func (c *Conn) resetBuffer() {
	c.wrBuf = c.wrBuf[:0]
}

func (c *Conn) appendType0(chunkID, tagType byte, timeMS, size uint32, payload []byte) {
	// TODO: timeMS more than 24 bit
	c.wrBuf = append(c.wrBuf,
		chunkID,
		byte(timeMS>>16), byte(timeMS>>8), byte(timeMS),
		byte(size>>16), byte(size>>8), byte(size),
		tagType,
		c.streamID, 0, 0, 0, // little endian streamID
	)
	c.wrBuf = append(c.wrBuf, payload...)
}

func (c *Conn) appendType3(chunkID byte, payload []byte) {
	c.wrBuf = append(c.wrBuf, 3<<6|chunkID)
	c.wrBuf = append(c.wrBuf, payload...)
}

func (c *Conn) writePacketSize() error {
	b := binary.BigEndian.AppendUint32(nil, c.wrPacketSize)
	return c.writeMessage(2, TypeSetPacketSize, 0, b)
}

func (c *Conn) writeConnect() error {
	b := amf.EncodeItems("connect", 1, map[string]any{
		"app":      c.App,
		"flashVer": "FMLE/3.0 (compatible; FMSc/1.0)",
		"tcUrl":    c.url,
	})
	if err := c.writeMessage(3, TypeCommand, 0, b); err != nil {
		return err
	}

	v, err := c.readResponse(1)
	if err != nil {
		return err
	}

	code := getString(v, 3, "code")
	if code != "NetConnection.Connect.Success" {
		return fmt.Errorf("rtmp: wrong response %#v", v)
	}

	return nil
}

func (c *Conn) writeReleaseStream() error {
	b := amf.EncodeItems("releaseStream", 2, nil, c.Stream)
	if err := c.writeMessage(3, TypeCommand, 0, b); err != nil {
		return err
	}
	b = amf.EncodeItems("FCPublish", 3, nil, c.Stream)
	if err := c.writeMessage(3, TypeCommand, 0, b); err != nil {
		return err
	}
	return nil
}
func (c *Conn) writeCreateStream() error {
	b := amf.EncodeItems("createStream", 4, nil)
	if err := c.writeMessage(3, TypeCommand, 0, b); err != nil {
		return err
	}

	v, err := c.readResponse(4)
	if err != nil {
		return err
	}

	if len(v) == 4 {
		if f, ok := v[3].(float64); ok {
			c.streamID = byte(f)
			return nil
		}
	}

	return fmt.Errorf("rtmp: wrong response %#v", v)
}

func (c *Conn) writePublish() error {
	b := amf.EncodeItems("publish", 5, nil, c.Stream, "live")
	if err := c.writeMessage(3, TypeCommand, 0, b); err != nil {
		return err
	}

	v, err := c.readResponse(0)
	if err != nil {
		return nil
	}

	code := getString(v, 3, "code")
	if code != "NetStream.Publish.Start" {
		return fmt.Errorf("rtmp: wrong response %#v", v)
	}

	return nil
}

func (c *Conn) writePlay() error {
	b := amf.EncodeItems("play", 5, nil, c.Stream)
	if err := c.writeMessage(3, TypeCommand, 0, b); err != nil {
		return err
	}

	v, err := c.readResponse(0)
	if err != nil {
		return nil
	}

	code := getString(v, 3, "code")
	if !strings.HasPrefix(code, "NetStream.Play.") {
		return fmt.Errorf("rtmp: wrong response %#v", v)
	}

	return nil
}

func (c *Conn) readSize(n uint32) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadAtLeast(c.rd, b, int(n)); err != nil {
		return nil, err
	}
	return b, nil
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

func getString(v []any, i int, key string) string {
	if len(v) <= i {
		return ""
	}
	if v, ok := v[i].(map[string]any); ok {
		if s, ok := v[key].(string); ok {
			return s
		}
	}
	return ""
}
