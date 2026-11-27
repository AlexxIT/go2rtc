package tutk

import (
	"bytes"
	"encoding/binary"
	"math/rand"
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

// remoteServers are the Xiaomi CN P2P servers seen in a Mi Home capture. Besides the one in the
// stream URL, each is asked for the camera's address; the master (10240) lookup could supply these later.
var remoteServers = []string{"42.157.162.73", "110.43.39.150", "110.43.68.243"}

// connectRemote follows the Mi Home app (TUTK SDK 3.3.0.7). c.addr is the P2P server.
//  1. 0x8003 -> 0x8004: our public address
//  2. every second: 0x0302 (where is UID) + 0x0205 (push me to the camera) to the server,
//     0x0401 to the last camera address the server gave (0x0103). The camera re-registers
//     with a new NAT port after each session, so the address can change while we wait.
//  3. camera answers 0x0401 -> we send 0x0402 -> camera 0x0404 -> we send 0x0404.
func (c *Conn) connectRemote(uid string, sid []byte) error {
	msg := func(n int, typ string) []byte {
		b := make([]byte, n)
		copy(b, "\x04\x02\x19\x02")
		binary.LittleEndian.PutUint16(b[4:], uint16(n-16))
		copy(b[8:], typ)
		if n >= 36 {
			copy(b[16:], uid)
		}
		return b
	}
	stage := func(st byte) []byte {
		b := msg(52, string([]byte{st, 4, 0x33}))
		copy(b[36:], sid)
		copy(b[48:], "\x07\x00\x03\x03")
		return b
	}

	q := msg(24, "\x03\x80\x3f")
	copy(q[20:], "\xf4\xfe")
	res, err := writeAndWait(c, func(r []byte) bool { return r[8] == 0x04 && r[9] == 0x80 && len(r) >= 24 }, q)
	if err != nil {
		return err
	}
	pub := append([]byte(nil), res[16:24]...)

	where := msg(288, "\x03\x02\x34")
	binary.BigEndian.PutUint16(where[38:], uint16(c.LocalAddr().(*net.UDPAddr).Port))
	copy(where[100:], sid)
	where[108] = stageDirect
	copy(where[112:], pub)

	wait := msg(48, "\x05\x02\x24")
	copy(wait[36:], sid)
	copy(wait[44:], "\x60\xea")

	servers := []*net.UDPAddr{{IP: c.addr.IP, Port: c.addr.Port}}
	for _, ip := range remoteServers {
		if !c.addr.IP.Equal(net.ParseIP(ip)) {
			servers = append(servers, &net.UDPAddr{IP: net.ParseIP(ip), Port: 10001})
		}
	}
	status := msg(36, "\x11\x02\x24")
	for _, s := range servers {
		_, _ = c.UDPConn.WriteToUDP(TransCodePartial(nil, status), s)
	}
	var camera *net.UDPAddr
	write := func(b []byte, to *net.UDPAddr) {
		_, _ = c.UDPConn.WriteToUDP(TransCodePartial(nil, b), to)
	}

	end := time.Now().Add(3 * time.Minute)
	_ = c.UDPConn.SetWriteDeadline(end)
	next := time.Now()
	buf := make([]byte, 1200)
	for st := byte(1); ; {
		if now := time.Now(); now.After(next) {
			next = now.Add(time.Second)
			// Servers answer 0x0302 once per port, so like the app also ask on a random
			// port 10000-10299 to keep getting the camera's current address.
			port := 10000 + rand.Intn(300)
			for i, s := range servers {
				write(where, s)
				write(where, &net.UDPAddr{IP: s.IP, Port: port + i})
				write(wait, s)
			}
			if camera != nil {
				write(stage(st), camera)
			}
		}

		_ = c.UDPConn.SetReadDeadline(next)
		n, from, err := c.UDPConn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() && time.Now().Before(end) {
				continue
			}
			return err
		}
		if n < 16 {
			continue
		}
		r := ReverseTransCodePartial(nil, buf[:n])

		switch {
		case r[8] == 0x01 && r[9] == 0x03 && n >= 44: // server: camera address
			addr := &net.UDPAddr{IP: net.IPv4(r[40], r[41], r[42], r[43]), Port: int(binary.BigEndian.Uint16(r[38:]))}
			if camera == nil || camera.Port != addr.Port || !camera.IP.Equal(addr.IP) {
				camera = addr
				write(stage(st), camera)
			}
		case camera == nil || !from.IP.Equal(camera.IP):
		case st == 1 && r[8] == 0x01 && r[9] == 0x04:
			st = 2
			camera = from
			write(stage(2), camera)
		case st == 2 && r[8] == 0x04 && r[9] == 0x04:
			write(stage(4), from)
			c.addr = from
			c.ver = []byte{r[2]}
			_ = c.SetDeadline(time.Now().Add(15 * time.Second))
			return nil
		}
	}
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
