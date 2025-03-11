package rtmp

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/flv/amf"
)

func NewServer(conn net.Conn) (*Conn, error) {
	c := &Conn{
		conn: conn,
		rd:   bufio.NewReaderSize(conn, core.BufferSize),
		wr:   conn,

		chunks: map[uint8]*chunk{},

		rdPacketSize: 128,
		wrPacketSize: 4096,
	}

	if err := c.serverHandshake(); err != nil {
		return nil, err
	}
	if err := c.writePacketSize(); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Conn) serverHandshake() error {
	// based on https://rtmp.veriskope.com/docs/spec/
	_ = c.conn.SetDeadline(time.Now().Add(core.ConnDeadline))

	// read C0
	b := make([]byte, 1)
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}

	if b[0] != 3 {
		return errors.New("rtmp: wrong handshake")
	}

	// write S0
	if _, err := c.conn.Write([]byte{3}); err != nil {
		return err
	}

	b = make([]byte, 1536)

	// write S1
	tsS1 := nowMS()
	binary.BigEndian.PutUint32(b, tsS1)
	binary.BigEndian.PutUint32(b[4:], 0)
	_, _ = rand.Read(b[8:])
	if _, err := c.conn.Write(b); err != nil {
		return err
	}

	// read C1
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}

	// write S2
	tsS2 := nowMS()
	binary.BigEndian.PutUint32(b, tsS1)
	binary.BigEndian.PutUint32(b[4:], tsS2)
	if _, err := c.conn.Write(b); err != nil {
		return err
	}

	// read C2
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}

	_ = c.conn.SetDeadline(time.Time{})
	return nil
}

func (c *Conn) ReadCommands() error {
	for {
		msgType, _, b, err := c.readMessage()
		if err != nil {
			return err
		}

		//log.Printf("%d %.256x", msgType, b)

		switch msgType {
		case TypeSetPacketSize:
			c.rdPacketSize = binary.BigEndian.Uint32(b)
		case TypeCommand:
			if err = c.acceptCommand(b); err != nil {
				return err
			}

			if c.Intent != "" {
				return nil
			}
		}
	}
}

const (
	CommandConnect       = "connect"
	CommandReleaseStream = "releaseStream"
	CommandFCPublish     = "FCPublish"
	CommandCreateStream  = "createStream"
	CommandPublish       = "publish"
	CommandPlay          = "play"
)

func (c *Conn) acceptCommand(b []byte) error {
	items, err := amf.NewReader(b).ReadItems()
	if err != nil {
		return nil
	}

	//log.Printf("%#v", items)

	if len(items) < 2 {
		return fmt.Errorf("rtmp: read command %x", b)
	}

	cmd, ok := items[0].(string)
	if !ok {
		return fmt.Errorf("rtmp: read command %x", b)
	}

	tID, ok := items[1].(float64) // transaction ID
	if !ok {
		return fmt.Errorf("rtmp: read command %x", b)
	}

	switch cmd {
	case CommandConnect:
		if len(items) == 3 {
			if v, ok := items[2].(map[string]any); ok {
				c.App, _ = v["app"].(string)
			}
		}

		payload := amf.EncodeItems(
			"_result", tID,
			map[string]any{"fmsVer": "FMS/3,0,1,123"},
			map[string]any{"code": "NetConnection.Connect.Success"},
		)
		return c.writeMessage(3, TypeCommand, 0, payload)

	case CommandReleaseStream:
		// if app is empty - will use key as app
		if c.App == "" && len(items) == 4 {
			c.App, _ = items[3].(string)
		}

		payload := amf.EncodeItems("_result", tID, nil)
		return c.writeMessage(3, TypeCommand, 0, payload)

	case CommandFCPublish: // no response

	case CommandCreateStream:
		payload := amf.EncodeItems("_result", tID, nil, 1)
		return c.writeMessage(3, TypeCommand, 0, payload)

	case CommandPublish, CommandPlay: // response later
		c.Intent = cmd
		c.streamID = 1

	default:
		println("rtmp: unknown command: " + cmd)
	}

	return nil
}

func (c *Conn) WriteStart() error {
	var code string
	if c.Intent == CommandPublish {
		code = "NetStream.Publish.Start"
	} else {
		code = "NetStream.Play.Start"
	}

	payload := amf.EncodeItems("onStatus", 0, nil, map[string]any{"code": code})
	return c.writeMessage(3, TypeCommand, 0, payload)
}

func nowMS() uint32 {
	return uint32(time.Now().UnixNano() / int64(time.Millisecond))
}
