package rtmp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net"

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
	b := make([]byte, 1+1536)
	// read C0+C1
	if _, err := io.ReadFull(c.rd, b); err != nil {
		return err
	}
	// write S0+S1, skip random
	if _, err := c.conn.Write(b); err != nil {
		return err
	}
	// read S1, skip check
	if _, err := io.ReadFull(c.rd, make([]byte, 1536)); err != nil {
		return err
	}
	// write C1
	if _, err := c.conn.Write(b[1:]); err != nil {
		return err
	}
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

		if c.App == "" {
			return fmt.Errorf("rtmp: read command %x", b)
		}

		payload := amf.EncodeItems(
			"_result", tID,
			map[string]any{"fmsVer": "FMS/3,0,1,123"},
			map[string]any{"code": "NetConnection.Connect.Success"},
		)
		return c.writeMessage(3, TypeCommand, 0, payload)

	case CommandReleaseStream:
		payload := amf.EncodeItems("_result", tID, nil)
		return c.writeMessage(3, TypeCommand, 0, payload)

	case CommandCreateStream:
		payload := amf.EncodeItems("_result", tID, nil, 1)
		return c.writeMessage(3, TypeCommand, 0, payload)

	case CommandPublish, CommandPlay: // response later
		c.Intent = cmd
		c.streamID = 1

	case CommandFCPublish: // no response

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
