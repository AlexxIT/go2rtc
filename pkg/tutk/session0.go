package tutk

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"
)

func (c *Conn) connectDirect(uid string, sid []byte) error {
	res, err := writeAndWait(
		c, func(res []byte) bool { return bytes.Index(res, []byte("\x02\x06\x12\x00")) == 8 },
		ConnectByUID(stageBroadcast, uid, sid),
	)
	if err != nil {
		return err
	}

	n := len(res) // should be 200
	c.ver = []byte{res[2], res[n-13], res[n-14], res[n-15], res[n-16]}

	_, err = c.Write(ConnectByUID(stageDirect, uid, sid))
	return err
}

func (c *Conn) connectRemote(uid string, sid []byte) error {
	res, err := writeAndWait(
		c, func(res []byte) bool { return bytes.Index(res, []byte("\x01\x03\x43")) == 8 },
		ConnectByUID(stageGetRemoteIP, uid, sid),
	)
	if err != nil {
		return err
	}

	// Read real IP from cloud server response.
	// Important ot use net.IPv4 because slice will be 16 bytes.
	c.addr.IP = net.IPv4(res[40], res[41], res[42], res[43])
	c.addr.Port = int(binary.BigEndian.Uint16(res[38:]))

	res, err = writeAndWait(
		c, func(res []byte) bool { return bytes.Index(res, []byte("\x04\x04\x33")) == 8 },
		ConnectByUID(stageRemoteAck, uid, sid),
	)
	if err != nil {
		return err
	}

	if len(res) == 52 {
		c.ver = []byte{res[2], res[51], res[50], res[49], res[48]}
	} else {
		c.ver = []byte{res[2]}
	}

	_, err = c.Write(ConnectByUID(stageRemoteOK, uid, sid))
	return err
}

func (c *Conn) clientStart(username, password string) error {
	_, err := writeAndWait(
		c, func(res []byte) bool {
			return len(res) >= 84 && res[28] == 0 && (res[29] == 0x14 || res[29] == 0x21)
		},
		c.session.ClientStart(0, username, password),
		c.session.ClientStart(1, username, password),
	)
	return err
}

func writeAndWait(conn net.Conn, ok func(res []byte) bool, req ...[]byte) ([]byte, error) {
	var t *time.Timer
	t = time.AfterFunc(1, func() {
		for _, b := range req {
			if _, err := conn.Write(b); err != nil {
				return
			}
		}
		if t != nil {
			t.Reset(time.Second)
		}
	})
	defer t.Stop()

	buf := make([]byte, 1200)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			return nil, err
		}

		if ok(buf[:n]) {
			return buf[:n], nil
		}
	}
}

const (
	magic      = "\x04\x02\x19"     // include version 0x19
	sdkVersion = "\x06\x00\x03\x03" // 3.3.0.6
)

const (
	stageBroadcast = iota + 1
	stageDirect
	stageGetPublicIP
	stageGetRemoteIP
	stageRemoteReq
	stageRemoteAck
	stageRemoteOK
)

func ConnectByUID(stage byte, uid string, sid8 []byte) []byte {
	var b []byte

	switch stage {
	case stageBroadcast, stageDirect:
		b = make([]byte, 68)
		copy(b[8:], "\x01\x06\x21")
		copy(b[52:], sdkVersion)
		copy(b[56:], sid8)
		b[64] = stage // 1 or 2

	case stageGetPublicIP:
		b = make([]byte, 54)
		copy(b[8:], "\x07\x10\x18")

	case stageGetRemoteIP:
		b = make([]byte, 112)
		copy(b[8:], "\x03\x02\x34")
		copy(b[100:], sid8)
		b[108] = stageDirect

	case stageRemoteReq:
		b = make([]byte, 52)
		copy(b[8:], "\x01\x04\x33")
		copy(b[36:], sid8)
		copy(b[48:], sdkVersion)

	case stageRemoteAck:
		b = make([]byte, 44)
		copy(b[8:], "\x02\x04\x33")
		copy(b[36:], sid8)

	case stageRemoteOK:
		b = make([]byte, 52)
		copy(b[8:], "\x04\x04\x33")
		copy(b[36:], sid8)
		copy(b[48:], sdkVersion)
	}

	copy(b, magic)
	b[3] = 0x02 // connection stage
	binary.LittleEndian.PutUint16(b[4:], uint16(len(b))-16)
	copy(b[16:], uid)

	return b
}
